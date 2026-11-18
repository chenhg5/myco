package domain

import (
	"context"
	"regexp"
	"time"
)

// A page's data (templates.md): records in named collections, each with a
// version, changed one at a time by the people looking at the page (the
// App's myco bridge) and by agents (the CLI). Every change is also kept in
// the page's change feed, which is what an agent watches.

// PageRecord is one record of a page's collection.
type PageRecord struct {
	Page       string
	Collection string
	ID         string
	// Data is the record: a JSON object.
	Data      string
	Version   int
	UpdatedBy string
	UpdatedAt time.Time
	// Pos orders a collection's records (the order they were first put).
	Pos int64
}

// PageChange is one entry of a page's change feed.
type PageChange struct {
	Seq        int64
	Page       string
	Collection string
	Record     string
	// Op is "put" or "delete".
	Op string
	// Data is the record after a put ("" after a delete).
	Data    string
	Version int
	By      string
	At      time.Time
}

// PageDataRepository stores pages' records and change feeds.
type PageDataRepository interface {
	// Records lists a collection's records ("" for all), in their order.
	Records(ctx context.Context, page, collection string) ([]*PageRecord, error)
	Record(ctx context.Context, page, collection, id string) (*PageRecord, error)
	// Put writes a record when its version is still ifVersion (0: it must
	// not exist yet; -1: whatever it is), and adds the change; else
	// CodeConflict.
	Put(ctx context.Context, r *PageRecord, ifVersion int) (*PageChange, error)
	// Remove deletes a record (ifVersion as for Put) and adds the change.
	Remove(ctx context.Context, page, collection, id string, ifVersion int, by string, at time.Time) (*PageChange, error)
	// Changes lists a page's changes after seq, oldest first; oldest is
	// the first seq still kept (0 when none).
	Changes(ctx context.Context, page string, after int64, limit int) (changes []*PageChange, oldest int64, err error)
	// Seq is the page's latest change (0 when none).
	Seq(ctx context.Context, page string) (int64, error)
	Count(ctx context.Context, page string) (int, error)
	// Trim keeps a page's newest keep changes.
	Trim(ctx context.Context, page string, keep int) error
	DeletePage(ctx context.Context, page string) error
}

const (
	// MaxPageRecord bounds one record (its JSON).
	MaxPageRecord = 64 << 10
	// MaxPageRecords bounds how many records a page keeps.
	MaxPageRecords = 20000
	// PageChangesKept: older changes are let go (a watcher that fell
	// behind lists the records again).
	PageChangesKept = 5000
)

var (
	collectionName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)
	recordID       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// ParseCollection checks a collection's name.
func ParseCollection(s string) (string, error) {
	if !collectionName.MatchString(s) {
		return "", Errf(CodeInvalidArgument, "invalid collection %q", s).
			WithSuggestion("a collection is a short lowercase name, such as items or rows (letters, digits, - and _)")
	}
	return s, nil
}

// ParseRecordID checks a record's id.
func ParseRecordID(s string) (string, error) {
	if !recordID.MatchString(s) {
		return "", Errf(CodeInvalidArgument, "invalid record id %q", s).
			WithSuggestion("a record id is up to 64 letters, digits, - and _ (leave it out to get a new one)")
	}
	return s, nil
}
