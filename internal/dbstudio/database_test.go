package dbstudio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type memorySecrets struct {
	sync.Mutex
	values map[string]string
	fail   bool
}

func (s *memorySecrets) Get(k string) (string, error) {
	s.Lock()
	defer s.Unlock()
	v, ok := s.values[k]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (s *memorySecrets) Set(k, v string) error {
	s.Lock()
	defer s.Unlock()
	if s.fail {
		return errors.New("locked")
	}
	s.values[k] = v
	return nil
}
func (s *memorySecrets) Delete(k string) error {
	s.Lock()
	defer s.Unlock()
	delete(s.values, k)
	return nil
}
func TestDatabaseProfilesPrivateScopedAndVersioned(t *testing.T) {
	secrets := &memorySecrets{values: map[string]string{}}
	s := Store{Home: t.TempDir(), Scope: "hub\x00page", Secrets: secrets}
	pw := "private-test-password"
	in := ProfileInput{Profile: Profile{Name: "Local", Engine: "postgres", Host: "localhost", Port: 5432, User: "tester", TLS: "disable", ReadOnly: true}, Password: &pw}
	p, err := s.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasPassword {
		t.Fatal("missing credential status")
	}
	raw, _ := json.Marshal(p)
	file, _ := os.ReadFile(s.file())
	if strings.Contains(string(raw)+string(file), pw) {
		t.Fatal("credentials leaked into public metadata")
	}
	if runtime.GOOS != "windows" {
		stat, _ := os.Stat(s.file())
		if stat.Mode().Perm() != 0600 {
			t.Fatal("metadata permissions")
		}
	}
	other := s
	other.Scope = "hub\x00other"
	if _, err := other.Get(p.ID); err == nil {
		t.Fatal("profiles crossed page scopes")
	}
	in.Profile = p
	in.Name = "Renamed"
	in.Password = nil
	p2, err := s.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.Get(p2.ID)
	if c.secret.Password != pw {
		t.Fatal("lost saved credential")
	}
	if len(secrets.values) != 1 {
		t.Fatal("old credential revision was not removed")
	}
	if _, err = s.Save(in); err == nil {
		t.Fatal("stale connection overwrote a newer revision")
	}
	in.Profile = p2
	in.Host = "other.example"
	p3, err := s.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	if p3.HasPassword {
		t.Fatal("credential followed a different server")
	}
	if err = s.Forget(p3.ID, p3.Version); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.List()
	if len(rows) != 0 {
		t.Fatal("connection remained")
	}
}
func TestDatabaseStatementBoundaries(t *testing.T) {
	for _, s := range []string{"SELECT ';'", "/* before */ SELECT 1; -- end", "SELECT [semi;colon] FROM [a]", "WITH t AS (SELECT 1) SELECT * FROM t"} {
		if _, err := singleStatement(s, "sqlite"); err != nil {
			t.Errorf("rejected %q: %v", s, err)
		}
	}
	for _, s := range []string{"SELECT 1; DELETE FROM t", "SELECT 1; PRAGMA query_only=OFF", "SELECT '\\'; DELETE FROM t;--'", "SELECT /* unclosed", "/* nothing */", "SELECT 1 /*!; DELETE FROM t */"} {
		if _, err := singleStatement(s, "sqlite"); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"SELECT $$a;b$$", "SELECT $tag$a;b$tag$", `SELECT E'a\';b'`} {
		if _, err := singleStatement(s, "postgres"); err != nil {
			t.Errorf("postgres string rejected: %v", err)
		}
	}
	if queryStatement("UPDATE t SET x=1", "sqlite") == nil {
		t.Fatal("query accepted a write")
	}
}
func sqliteFixture(t *testing.T) Connection {
	t.Helper()
	path := filepath.Join(t.TempDir(), "example.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE things(id INTEGER PRIMARY KEY, name TEXT NOT NULL, number INTEGER, empty TEXT, data BLOB, note TEXT);INSERT INTO things VALUES(1,'hello',9007199254740993,'',x'00FF',NULL);CREATE TABLE unkeyed(name TEXT);INSERT INTO unkeyed VALUES('a')`)
	if err != nil {
		t.Fatal(err)
	}
	return Connection{Profile: Profile{ID: "sqlite", Engine: "sqlite", Path: path, ReadOnly: false}}
}
func TestDatabaseSQLiteCRUDPrecisionAndConflicts(t *testing.T) {
	c := sqliteFixture(t)
	ctx := context.Background()
	tables, err := c.Tables(ctx, Request{})
	if err != nil || len(tables) != 2 {
		t.Fatal(tables, err)
	}
	r := Request{Table: "things", Limit: 100, Action: "browse"}
	out, err := c.Browse(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 || out.ReadOnly {
		t.Fatal("browse contract")
	}
	row := out.Rows[0]
	if row.Values[2].Value != "9007199254740993" || row.Values[3].Kind != "text" || row.Values[3].Value != "" || row.Values[4].Kind != "binary" || row.Values[5].Kind != "null" {
		t.Fatal("lost typed values")
	}
	r.Action = "update"
	r.Key = row.Key
	r.Token = row.Token
	r.Values = map[string]Cell{"name": {Kind: "text", Value: "quote'; DELETE FROM things; --"}}
	if _, err = c.Mutate(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Mutate(ctx, r); err == nil {
		t.Fatal("stale edit was not refused")
	}
	out, err = c.Browse(ctx, Request{Table: "things"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Values[1].Value != "quote'; DELETE FROM things; --" {
		t.Fatal("value was interpolated as SQL")
	}
	r.Action = "insert"
	r.Key = nil
	r.Token = ""
	r.Values = map[string]Cell{"name": {Kind: "text", Value: "new"}, "note": {Kind: "text", Value: ""}}
	if _, err = c.Mutate(ctx, r); err != nil {
		t.Fatal(err)
	}
	out, _ = c.Browse(ctx, Request{Table: "things", Limit: 1})
	if len(out.Rows) != 1 || !out.Truncated {
		t.Fatal("pagination not bounded")
	}
	first := out.Rows[0]
	if _, err = c.Mutate(ctx, Request{Table: "things", Action: "delete", Key: first.Key, Token: first.Token}); err != nil {
		t.Fatal(err)
	}
	ro := c
	ro.ReadOnly = true
	if _, err = ro.SQL(ctx, Request{Mode: "execute", SQL: "DELETE FROM things"}); err == nil {
		t.Fatal("read-only execution allowed")
	}
	if _, err = c.SQL(ctx, Request{Mode: "query", SQL: "WITH x AS (SELECT 1) DELETE FROM things"}); err == nil {
		t.Fatal("read-only query allowed CTE write")
	}
	if _, err = c.SQL(ctx, Request{Mode: "query", SQL: "SELECT * FROM things; PRAGMA query_only=OFF; DELETE FROM things"}); err == nil {
		t.Fatal("multiple statements allowed")
	}
	if _, err = c.Mutate(ctx, Request{Table: "unkeyed", Action: "delete", Key: map[string]Cell{"name": {Kind: "text", Value: "a"}}, Token: "x"}); err == nil {
		t.Fatal("edited a row without a primary key")
	}
	if _, err = c.SQL(ctx, Request{Mode: "execute", SQL: "CREATE TABLE more(id INTEGER PRIMARY KEY)"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SQL(ctx, Request{Mode: "execute", SQL: "BEGIN TRANSACTION"}); err == nil {
		t.Fatal("pretended to retain a manual transaction between independent requests")
	}
	if _, err = c.SQL(ctx, Request{Mode: "execute", SQL: "CREATE TABLE linked(id INTEGER PRIMARY KEY,thing_id INTEGER REFERENCES things(id))"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Mutate(ctx, Request{Table: "linked", Action: "insert", Values: map[string]Cell{"thing_id": {Kind: "number", Value: "99999"}}}); err == nil {
		t.Fatal("SQLite row writes bypassed foreign-key constraints")
	}
}
func TestDatabaseMongoExtendedJSONAndIDs(t *testing.T) {
	d, err := document(`{"_id":{"$oid":"507f1f77bcf86cd799439011"},"large":{"$numberLong":"9007199254740993"}}`)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := docID(d)
	if !ok || sameBSON(id, "507f1f77bcf86cd799439011") {
		t.Fatal("ObjectID lost its type")
	}
	text, err := canonical(d)
	if err != nil || !strings.Contains(text, "9007199254740993") {
		t.Fatal("int64 lost precision")
	}
	for _, s := range []string{"null", "[]", "{broken}"} {
		if _, err = document(s); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}
