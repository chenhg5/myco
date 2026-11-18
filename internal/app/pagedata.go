package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// PageDataService: a page's records (templates.md). Whoever may see a page
// reads and writes its records, one record at a time, each with a version;
// every change goes into the page's change feed, which agents watch (and
// the App, for what others changed). A page made from a template checks
// records against its manifest's data structure.
type PageDataService struct {
	pages *PageService
	data  domain.PageDataRepository
	ids   IDGenerator
	clock Clock

	mu      sync.Mutex
	waiters map[string][]chan struct{}
}

// NewPageDataService wires pages' data.
func NewPageDataService(pages *PageService, data domain.PageDataRepository, ids IDGenerator, clock Clock) *PageDataService {
	return &PageDataService{pages: pages, data: data, ids: ids, clock: clock, waiters: map[string][]chan struct{}{}}
}

// schema is the page's data structure (nil: anything goes).
func schema(p *domain.Page) map[string]domain.ManifestCollection {
	if p.Manifest == "" {
		return nil
	}
	m, err := domain.ParseManifest([]byte(p.Manifest))
	if err != nil {
		return nil
	}
	return m.Data
}

func (s *PageDataService) page(ctx context.Context, by Actor, id string) (*domain.Page, error) {
	pv, err := s.pages.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	return pv.Page, nil
}

// Records lists a page's records (of one collection, or all) and the
// page's latest change.
func (s *PageDataService) Records(ctx context.Context, by Actor, page, collection string) ([]*domain.PageRecord, int64, error) {
	if _, err := s.page(ctx, by, page); err != nil {
		return nil, 0, err
	}
	if collection != "" {
		if _, err := domain.ParseCollection(collection); err != nil {
			return nil, 0, err
		}
	}
	seq, err := s.data.Seq(ctx, page)
	if err != nil {
		return nil, 0, err
	}
	recs, err := s.data.Records(ctx, page, collection)
	return recs, seq, err
}

// Record reads one.
func (s *PageDataService) Record(ctx context.Context, by Actor, page, collection, id string) (*domain.PageRecord, error) {
	if _, err := s.page(ctx, by, page); err != nil {
		return nil, err
	}
	return s.data.Record(ctx, page, collection, id)
}

// PutInput is a record to write: a new one (ID ""), or one by its id.
// IfVersion is the version read (0: it must be new); -1 writes whatever it
// is.
type PutInput struct {
	Collection string
	ID         string
	Data       string
	IfVersion  int
	By         string
}

// Put writes a record. On a conflict the latest record comes back with the
// error.
func (s *PageDataService) Put(ctx context.Context, by Actor, page string, in PutInput) (*domain.PageRecord, error) {
	p, err := s.page(ctx, by, page)
	if err != nil {
		return nil, err
	}
	if p.Live {
		return nil, domain.Errf(domain.CodeInvalidArgument, "page %s is a live page; it has no records", page)
	}
	coll, err := domain.ParseCollection(in.Collection)
	if err != nil {
		return nil, err
	}
	if len(in.Data) > domain.MaxPageRecord {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the record is larger than %d KB", domain.MaxPageRecord>>10).
			WithSuggestion("keep files as attachments and put their ids in the record")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(in.Data), &obj); err != nil || obj == nil {
		return nil, domain.Errf(domain.CodeInvalidArgument, "a record is a JSON object, such as {\"title\": \"…\"}")
	}
	if sc := schema(p); sc != nil {
		col, ok := sc[coll]
		if !ok {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this page has no collection %q", coll).
				WithSuggestion("its template's myco.json says which collections it has (myco page data list shows them)")
		}
		if err := col.CheckRecord(coll, in.Data); err != nil {
			return nil, err
		}
		if col.Single {
			if in.ID == "" {
				in.ID = "main"
			}
			if in.ID != "main" {
				return nil, domain.Errf(domain.CodeInvalidArgument, "%s holds one record, main", coll)
			}
		}
	}
	id := in.ID
	if id == "" {
		id = s.ids.New("r")
		in.IfVersion = 0
	} else if id, err = domain.ParseRecordID(id); err != nil {
		return nil, err
	}
	if n, err := s.data.Count(ctx, page); err != nil {
		return nil, err
	} else if n >= domain.MaxPageRecords {
		if _, e := s.data.Record(ctx, page, coll, id); e != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this page already has %d records", domain.MaxPageRecords).
				WithSuggestion("delete records no longer needed")
		}
	}
	rec := &domain.PageRecord{Page: page, Collection: coll, ID: id, Data: in.Data, UpdatedBy: in.By, UpdatedAt: s.clock.Now()}
	ch, err := s.data.Put(ctx, rec, in.IfVersion)
	if err != nil {
		if domain.IsCode(err, domain.CodeConflict) {
			if cur, e := s.data.Record(ctx, page, coll, id); e == nil {
				return cur, err
			}
		}
		return nil, err
	}
	s.changed(ctx, page, ch.Seq)
	return rec, nil
}

// Remove deletes a record (ifVersion as for Put).
func (s *PageDataService) Remove(ctx context.Context, by Actor, page, collection, id string, ifVersion int, byName string) error {
	if _, err := s.page(ctx, by, page); err != nil {
		return err
	}
	ch, err := s.data.Remove(ctx, page, collection, id, ifVersion, byName, s.clock.Now())
	if err != nil {
		return err
	}
	s.changed(ctx, page, ch.Seq)
	return nil
}

// ChangeList is what changed after a point of the feed. Reset: the feed
// no longer goes back that far; list the records again from Seq.
type ChangeList struct {
	Changes []*domain.PageChange
	Seq     int64
	Reset   bool
}

// Changes lists a page's changes after seq; with wait, it waits that long
// for one when there is none yet.
func (s *PageDataService) Changes(ctx context.Context, by Actor, page string, after int64, wait time.Duration) (*ChangeList, error) {
	if _, err := s.page(ctx, by, page); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		ch := s.listen(page)
		changes, oldest, err := s.data.Changes(ctx, page, after, 500)
		if err != nil {
			s.unlisten(page, ch)
			return nil, err
		}
		seq, err := s.data.Seq(ctx, page)
		if err != nil {
			s.unlisten(page, ch)
			return nil, err
		}
		if after > 0 && oldest > after+1 {
			s.unlisten(page, ch)
			return &ChangeList{Changes: []*domain.PageChange{}, Seq: seq, Reset: true}, nil
		}
		if len(changes) > 0 || wait <= 0 || time.Now().After(deadline) {
			s.unlisten(page, ch)
			if len(changes) > 0 {
				seq = changes[len(changes)-1].Seq
			}
			return &ChangeList{Changes: changes, Seq: max(seq, after)}, nil
		}
		t := time.NewTimer(time.Until(deadline))
		select {
		case <-ch:
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			s.unlisten(page, ch)
			return nil, ctx.Err()
		}
		t.Stop()
		s.unlisten(page, ch)
	}
}

func (s *PageDataService) listen(page string) chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.waiters[page] = append(s.waiters[page], ch)
	s.mu.Unlock()
	return ch
}

func (s *PageDataService) unlisten(page string, ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.waiters[page]
	for i, c := range list {
		if c == ch {
			s.waiters[page] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(s.waiters[page]) == 0 {
		delete(s.waiters, page)
	}
}

func (s *PageDataService) changed(ctx context.Context, page string, seq int64) {
	s.mu.Lock()
	for _, c := range s.waiters[page] {
		select {
		case c <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	// Older changes are let go now and then.
	if seq%200 == 0 {
		_ = s.data.Trim(context.WithoutCancel(ctx), page, domain.PageChangesKept)
	}
}

// Seed puts a template's sample records into a new page.
func (s *PageDataService) Seed(ctx context.Context, by Actor, page string, sample map[string][]json.RawMessage, byName string) error {
	for coll, recs := range sample {
		for _, raw := range recs {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(raw, &obj); err != nil {
				return domain.Errf(domain.CodeInvalidArgument, "sample.json: %s holds objects", coll)
			}
			id := ""
			if v, ok := obj["id"]; ok {
				_ = json.Unmarshal(v, &id)
				delete(obj, "id")
			}
			data, _ := json.Marshal(obj)
			if _, err := s.Put(ctx, by, page, PutInput{Collection: coll, ID: id, Data: string(data), IfVersion: -1, By: byName}); err != nil {
				return err
			}
		}
	}
	return nil
}
