package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/myco-ai/myco/internal/domain"
)

func (r *PageAttachmentRepo) Candidates(ctx context.Context, project string) ([]domain.PageAttachmentMigration, []domain.PageAttachmentMigrationIssue, error) {
	return attachmentCandidates(ctx, r.db, project)
}

// References are derived from schema, not application/template identities.
func attachmentCandidates(ctx context.Context, q sqlQueryer, project string) ([]domain.PageAttachmentMigration, []domain.PageAttachmentMigrationIssue, error) {
	aliases := map[string]bool{}
	aliasRows, err := q.QueryContext(ctx, `SELECT page,name FROM page_attachment_aliases`)
	if err != nil {
		return nil, nil, internal(err)
	}
	for aliasRows.Next() {
		var p, n string
		if err = aliasRows.Scan(&p, &n); err != nil {
			aliasRows.Close()
			return nil, nil, internal(err)
		}
		aliases[p+"\x00"+n] = true
	}
	err = aliasRows.Err()
	aliasRows.Close()
	if err != nil {
		return nil, nil, internal(err)
	}
	rows, err := q.QueryContext(ctx, `SELECT p.id,p.name,d.name,d.body,d.content_type,d.size,d.blob,d.updated_by,d.updated_at FROM project_docs d JOIN projects p ON p.id=d.project WHERE (?='' OR p.name=?) ORDER BY p.name,d.name`, project, project)
	if err != nil {
		return nil, nil, internal(err)
	}
	docs := map[string]*domain.PageAttachmentMigration{}
	byBlob := map[string]*domain.PageAttachmentMigration{}
	for rows.Next() {
		var x domain.PageAttachmentMigration
		var at int64
		if err = rows.Scan(&x.ProjectID, &x.ProjectName, &x.Doc.Name, &x.Doc.Body, &x.Doc.ContentType, &x.Doc.Size, &x.Doc.Blob, &x.Doc.UpdatedBy, &at); err != nil {
			rows.Close()
			return nil, nil, internal(err)
		}
		x.Doc.Project = x.ProjectID
		x.Doc.UpdatedAt = fromMS(at)
		key := x.ProjectName + "\x00" + x.Doc.Name
		docs[key] = &x
		if x.Doc.Blob != "" {
			blobKey := x.ProjectName + "\x00" + x.Doc.Blob
			if _, seen := byBlob[blobKey]; seen {
				byBlob[blobKey] = nil
			} else {
				byBlob[blobKey] = &x
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, internal(err)
	}
	// A legacy filename can look like another document's blob ID. Refuse
	// that ambiguous namespace rather than silently binding either original.
	ambiguous := map[*domain.PageAttachmentMigration]bool{}
	for _, source := range docs {
		if source.Doc.Blob == "" {
			continue
		}
		if named := docs[source.ProjectName+"\x00"+source.Doc.Blob]; named != nil && named != source {
			ambiguous[source], ambiguous[named] = true, true
		}
	}
	rows, err = q.QueryContext(ctx, `SELECT p.id,p.project,p.manifest,r.collection,r.id,r.data,r.version FROM page_records r JOIN pages p ON p.id=r.page WHERE (?='' OR p.project=?) ORDER BY p.id,r.collection,r.id`, project, project)
	if err != nil {
		return nil, nil, internal(err)
	}
	signatures := map[*domain.PageAttachmentMigration]string{}
	targetSets := map[*domain.PageAttachmentMigration]map[string]string{}
	issues := []domain.PageAttachmentMigrationIssue{}
	for rows.Next() {
		var page, proj, manifest, collection, id, data string
		var version int
		if err = rows.Scan(&page, &proj, &manifest, &collection, &id, &data, &version); err != nil {
			rows.Close()
			return nil, nil, internal(err)
		}
		refs := domain.PageAttachmentReferences(manifest, collection, data)
		sort.Slice(refs, func(i, j int) bool { return refs[i].Field < refs[j].Field })
		for _, ref := range refs {
			if !ref.Legacy && domain.IsPageAttachment(ref.Value) {
				continue
			}
			source := docs[proj+"\x00"+ref.Value]
			if source == nil && !ref.Legacy {
				source = byBlob[proj+"\x00"+ref.Value]
			}
			if source == nil || ambiguous[source] {
				if source == nil && aliases[page+"\x00"+ref.Value] {
					continue
				}
				if len(issues) < 1000 {
					issues = append(issues, domain.PageAttachmentMigrationIssue{Page: page, Collection: collection, Record: id, Field: ref.Field, Name: ref.Value, Reason: "no unique matching document in the page's project"})
				}
				continue
			}
			if targetSets[source] == nil {
				targetSets[source] = map[string]string{}
			}
			if _, ok := targetSets[source][page]; !ok {
				targetSets[source][page] = collection
			}
			// Includes content, not only versions: delete/recreate may reuse version 1.
			snap, _ := json.Marshal([]any{page, proj, manifest, collection, id, version, data, ref.Field})
			signatures[source] += string(snap) + "\n"
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, internal(err)
	}
	out := []domain.PageAttachmentMigration{}
	for source, targets := range targetSets {
		body, _ := json.Marshal(source.Doc)
		sum := sha256.Sum256(append(body, []byte(signatures[source])...))
		source.Signature = hex.EncodeToString(sum[:])
		for page, collection := range targets {
			source.Targets = append(source.Targets, domain.PageAttachmentTarget{Page: page, Collection: collection})
		}
		sort.Slice(source.Targets, func(i, j int) bool { return source.Targets[i].Page < source.Targets[j].Page })
		out = append(out, *source)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectName == out[j].ProjectName {
			return out[i].Doc.Name < out[j].Doc.Name
		}
		return out[i].ProjectName < out[j].ProjectName
	})
	return out, issues, nil
}

func (r *PageAttachmentRepo) CommitMigration(ctx context.Context, source domain.PageAttachmentMigration, files []*domain.PageAttachment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	current, _, err := attachmentCandidates(ctx, tx, source.ProjectName)
	if err != nil {
		return err
	}
	var found bool
	for _, x := range current {
		if x.Doc.Name == source.Doc.Name && x.ProjectID == source.ProjectID && x.Signature == source.Signature {
			found = true
			break
		}
	}
	if !found || len(files) != len(source.Targets) {
		return domain.Errf(domain.CodeConflict, "project document or page references changed during migration")
	}
	for i, a := range files {
		if a.Page != source.Targets[i].Page {
			return domain.Errf(domain.CodeConflict, "migration target changed")
		}
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT sha256 FROM page_attachments WHERE id=? AND page=?`, a.ID, a.Page).Scan(&stored); err != nil || stored != a.SHA256 {
			return domain.Errf(domain.CodeConflict, "migration staging content changed")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE page_attachments SET state='ready',expires_at=0 WHERE id=?`, a.ID); err != nil {
			return internal(err)
		}

		names := []string{source.Doc.Name}
		if source.Doc.Blob != "" {
			names = append(names, source.Doc.Blob)
		}
		for _, name := range names {
			if _, err = tx.ExecContext(ctx, `INSERT INTO page_attachment_aliases(page,project,name,attachment) VALUES(?,?,?,?) ON CONFLICT(page,name) DO UPDATE SET attachment=excluded.attachment,project=excluded.project`, a.Page, source.ProjectName, name, a.ID); err != nil {
				return internal(err)
			}
		}
		if err = rebindPageAttachments(ctx, tx, a.Page); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM project_docs WHERE project=? AND name=?`, source.ProjectID, source.Doc.Name); err != nil {
		return internal(err)
	}
	if source.Doc.Blob != "" {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO page_attachment_garbage(blob) SELECT ? WHERE NOT EXISTS(SELECT 1 FROM project_docs WHERE blob=?)`, source.Doc.Blob, source.Doc.Blob); err != nil {
			return internal(err)
		}
	}
	return internal(tx.Commit())
}
