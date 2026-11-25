package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func pageFileCommand() *Command {
	page := Flag{Name: "page", Kind: String, Required: true, Help: "Page id (pg_...)"}
	admin := Flag{Name: "admin-token-file", Kind: String, Help: "File containing a person's admin token (agents use their own identity)"}
	file := Flag{Name: "file", Kind: String, Required: true, Help: "Page attachment id (paf_...) or a legacy name for get"}
	list := &Command{Name: "list", Summary: "List this page's own files and quota (not project files)", Flags: []Flag{page, admin, {Name: "limit", Kind: Int, Default: "100", Help: "Results per page (1..1000)"}, {Name: "after", Kind: String, Help: "Cursor returned by the preceding list"}}, Run: handler(func(e *env) error {
		if err := e.in.Require("page"); err != nil {
			return err
		}
		if n := e.in.Int("limit"); n < 1 || n > 1000 {
			return errorf("invalid_argument", ExitUsage, "", "limit must be 1..1000")
		}
		c, err := e.projectClient()
		if err != nil {
			return err
		}
		out, err := c.PageAttachments(e.in.Ctx, e.in.String("page"), e.in.String("after"), e.in.Int("limit"))
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) {
			fmt.Fprintf(w, "Files %d/%d · bytes %d/%d · each at most %d bytes\n", out.Usage.FilesUsed, out.Usage.FilesMax, out.Usage.BytesUsed, out.Usage.BytesMax, out.Usage.MaxFileBytes)
			tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tBYTES\tSTATE\tCOLLECTION")
			for _, a := range out.Attachments {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", a.ID, a.Name, a.Size, a.State, a.Collection)
			}
			tw.Flush()
			if out.Next != "" {
				fmt.Fprintf(w, "Next: --after %s\n", out.Next)
			}
		})
	})}
	put := &Command{Name: "put", Summary: "Upload a page-owned file (associate it with a record within 24 hours)", Mutating: true, Flags: []Flag{page, admin, {Name: "path", Kind: String, Required: true, Help: "Local file, at most 20 MiB"}, {Name: "collection", Kind: String, Required: true, Help: "Collection whose write permissions govern the file"}, {Name: "as", Kind: String, Help: "Display name (default: local basename)"}, {Name: "key", Kind: String, Help: "Idempotency key (default: hash of page, collection, name and content)"}, {Name: "legacy-name", Kind: String, Help: "Optional compatibility name for an old page's file reference"}}, Run: handler(func(e *env) error {
		if err := e.in.Require("page", "path", "collection"); err != nil {
			return err
		}
		f, err := os.Open(e.in.String("path"))
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 20<<20+1))
		if err != nil {
			return err
		}
		if len(b) > 20<<20 {
			return errorf("invalid_argument", ExitUsage, "", "page files are limited to 20 MiB")
		}
		name := e.in.String("as")
		if name == "" {
			name = filepath.Base(e.in.String("path"))
		}
		key := e.in.String("key")
		if key == "" {
			h := sha256.New()
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", e.in.String("page"), e.in.String("collection"), name, e.in.String("legacy-name"))
			h.Write(b)
			key = "cli:" + hex.EncodeToString(h.Sum(nil))
		}
		if e.in.Bool("dry-run") {
			return e.dryRun(map[string]any{"action": "page_file_put", "page": e.in.String("page"), "collection": e.in.String("collection"), "name": name, "bytes": len(b), "key": key})
		}
		c, err := e.projectClient()
		if err != nil {
			return err
		}
		a, err := c.PutPageAttachment(e.in.Ctx, client.PageAttachmentUpload{Page: e.in.String("page"), Collection: e.in.String("collection"), Name: name, ContentType: http.DetectContentType(b), Key: key, LegacyName: e.in.String("legacy-name")}, bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return err
		}
		return e.p.Print(a, func(w io.Writer) { fmt.Fprintf(w, "%s  %s (%d bytes, %s)\n", a.ID, a.Name, a.Size, a.State) })
	})}
	get := &Command{Name: "get", Summary: "Download and verify a page attachment", Flags: []Flag{page, file, admin, {Name: "out", Kind: String, Required: true, Help: "Destination file"}}, Run: handler(func(e *env) error {
		if err := e.in.Require("page", "file", "out"); err != nil {
			return err
		}
		c, err := e.projectClient()
		if err != nil {
			return err
		}
		var a *api.PageAttachment
		id := e.in.String("file")
		if len(id) > 4 && id[:4] == "paf_" {
			a, err = c.PageAttachment(e.in.Ctx, e.in.String("page"), id)
		} else {
			a, err = c.ResolvePageAttachment(e.in.Ctx, e.in.String("page"), id)
		}
		if err != nil {
			return err
		}
		response, err := c.OpenPageAttachment(e.in.Ctx, a.Page, a.ID, "")
		if err != nil {
			return err
		}
		defer response.Body.Close()
		b, err := io.ReadAll(io.LimitReader(response.Body, 20<<20+1))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if int64(len(b)) != a.Size || hex.EncodeToString(sum[:]) != a.SHA256 {
			return errorf("invalid_state", ExitError, "try downloading again", "page attachment content verification failed")
		}
		if err = os.WriteFile(e.in.String("out"), b, 0600); err != nil {
			return err
		}
		return e.p.Print(map[string]any{"file_id": a.ID, "path": e.in.String("out"), "bytes": len(b), "sha256": a.SHA256}, func(w io.Writer) { fmt.Fprintln(w, e.in.String("out")) })
	})}
	remove := &Command{Name: "remove", Summary: "Remove an unreferenced page attachment", Mutating: true, Flags: []Flag{page, file, admin}, Run: handler(func(e *env) error {
		if err := e.in.Require("page", "file"); err != nil {
			return err
		}
		if e.in.Bool("dry-run") {
			return e.dryRun(map[string]any{"action": "page_file_remove", "page": e.in.String("page"), "file": e.in.String("file")})
		}
		c, err := e.projectClient()
		if err != nil {
			return err
		}
		if err = c.DeletePageAttachment(e.in.Ctx, e.in.String("page"), e.in.String("file")); err != nil {
			return err
		}
		return e.p.Print(map[string]bool{"deleted": true}, func(w io.Writer) { fmt.Fprintln(w, "removed") })
	})}
	migrate := &Command{Name: "migrate", Summary: "Move referenced project documents into page-owned attachments (workspace owner/admin)", Mutating: true, Flags: []Flag{admin, {Name: "project", Kind: String, Help: "Only this project (default: all)"}, {Name: "limit", Kind: Int, Default: "25", Help: "Source documents per batch (1..100); repeat while remaining is nonzero"}}, Run: handler(func(e *env) error {
		if e.in.Int("limit") < 1 || e.in.Int("limit") > 100 {
			return errorf("invalid_argument", ExitUsage, "", "limit must be 1..100")
		}
		c, err := e.adminClient()
		if err != nil {
			return err
		}
		out, err := c.MigratePageAttachments(e.in.Ctx, api.PageAttachmentMigrationRequest{Project: e.in.String("project"), DryRun: e.in.Bool("dry-run"), Limit: e.in.Int("limit")})
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) {
			fmt.Fprintf(w, "Sources %d · moved %d · remaining %d · unmatched %d\n", out.Sources, out.Moved, out.Remaining, len(out.Skipped))
			for _, r := range out.Results {
				fmt.Fprintf(w, "%s/%s\t%d bytes\t%s\t%s\n", r.Project, r.Name, r.Size, r.SHA256, r.Error)
			}
			for _, x := range out.Skipped {
				fmt.Fprintf(w, "unmatched %s/%s/%s.%s: %s\n", x.Page, x.Collection, x.Record, x.Field, x.Reason)
			}
		})
	})}
	return (&Command{Name: "file", Summary: "Files owned by a page; separate from project context and message attachments"}).Add(list, put, get, remove, migrate)
}
