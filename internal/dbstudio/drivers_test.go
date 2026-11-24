package dbstudio

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in acceptance uses only isolated fixtures created for this test, never a
// user's configured connection. The private fixture file is not printed.
func TestDatabaseDrivers(t *testing.T) {
	file := os.Getenv("DBSTUDIO_ACCEPTANCE_FILE")
	if file == "" {
		t.Skip("set DBSTUDIO_ACCEPTANCE_FILE to isolated driver fixtures")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("could not read fixture configuration")
	}
	var fixtures []struct {
		Name     string  `json:"name"`
		Profile  Profile `json:"profile"`
		Password string  `json:"password"`
	}
	if json.Unmarshal(raw, &fixtures) != nil {
		t.Fatal("invalid fixture configuration")
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			c := Connection{Profile: fixture.Profile, secret: credentials{Password: fixture.Password}}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			if err := c.Ping(ctx); err != nil {
				t.Fatal(err)
			}
			if c.Engine == "mongodb" {
				testMongoDriver(t, ctx, c)
				return
			}
			create := "CREATE TABLE studio_acceptance(id BIGINT PRIMARY KEY, name VARCHAR(100), note TEXT)"
			if _, err := c.SQL(ctx, Request{Mode: "execute", SQL: create}); err != nil {
				t.Fatal(err)
			}
			defer c.SQL(context.Background(), Request{Mode: "execute", SQL: "DROP TABLE studio_acceptance"})
			if _, err := c.Mutate(ctx, Request{Table: "studio_acceptance", Schema: func() string {
				if c.Engine == "postgres" {
					return "public"
				}
				return c.Database
			}(), Action: "insert", Values: map[string]Cell{"id": {Kind: "number", Value: "9007199254740993"}, "name": {Kind: "text", Value: "first"}, "note": {Kind: "null"}}}); err != nil {
				t.Fatal(err)
			}
			r := Request{Table: "studio_acceptance"}
			if c.Engine == "postgres" {
				r.Schema = "public"
			}
			result, err := c.Browse(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 1 || result.Rows[0].Values[0].Value != "9007199254740993" {
				t.Fatal("BIGINT or row count mismatch")
			}
			row := result.Rows[0]
			r.Action = "update"
			r.Key = row.Key
			r.Token = row.Token
			r.Values = map[string]Cell{"name": {Kind: "text", Value: "second"}}
			if _, err = c.Mutate(ctx, r); err != nil {
				t.Fatal(err)
			}
			if _, err = c.Mutate(ctx, r); err == nil {
				t.Fatal("stale row edit succeeded")
			}
			if _, err = c.SQL(ctx, Request{Mode: "query", SQL: "WITH x AS (SELECT 1) SELECT * FROM studio_acceptance"}); err != nil {
				t.Fatal(err)
			}
			if _, err = c.SQL(ctx, Request{Mode: "query", SQL: "WITH x AS (SELECT 1) DELETE FROM studio_acceptance"}); err == nil {
				t.Fatal("read-only query wrote data")
			}
			ro := c
			ro.ReadOnly = true
			if _, err = ro.SQL(ctx, Request{Mode: "execute", SQL: "DELETE FROM studio_acceptance"}); err == nil {
				t.Fatal("read-only profile wrote data")
			}
			result, err = c.Browse(ctx, Request{Table: r.Table, Schema: r.Schema})
			if err != nil {
				t.Fatal(err)
			}
			r.Key = result.Rows[0].Key
			r.Token = result.Rows[0].Token
			r.Action = "delete"
			if _, err = c.Mutate(ctx, r); err != nil {
				t.Fatal(err)
			}
			probe := "SELECT pg_sleep(5)"
			if c.Engine == "mysql" {
				probe = "SELECT SLEEP(5)"
			}
			short, stop := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer stop()
			_, err = c.SQL(short, Request{Mode: "query", SQL: probe})
			if err == nil {
				t.Fatal("timed-out query succeeded")
			}
		})
	}
}
func testMongoDriver(t *testing.T, ctx context.Context, c Connection) {
	t.Helper()
	r := Request{Table: "studio_acceptance", Action: "insert", Document: `{"_id":{"$oid":"507f1f77bcf86cd799439011"},"name":"first","large":{"$numberLong":"9007199254740993"}}`}
	if _, err := c.Mongo(ctx, r); err != nil {
		t.Fatal(err)
	}
	result, err := c.Mongo(ctx, Request{Table: r.Table, Action: "find", Query: `{}`})
	if err != nil || len(result.Rows) != 1 {
		t.Fatal("find failed", err)
	}
	row := result.Rows[0]
	if !strings.Contains(row.Document, "9007199254740993") {
		t.Fatal("MongoDB int64 precision lost")
	}
	r.Action = "replace"
	r.Original = row.Document
	r.Token = row.Token
	r.Document = strings.Replace(row.Document, "first", "second", 1)
	if _, err = c.Mongo(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Mongo(ctx, r); err == nil {
		t.Fatal("stale document replaced a newer document")
	}
	if _, err = c.Mongo(ctx, Request{Table: r.Table, Action: "aggregate", Query: `[{"$match":{}},{"$project":{"name":1}}]`}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Mongo(ctx, Request{Table: r.Table, Action: "aggregate", Query: `[{"$out":"other"}]`}); err == nil {
		t.Fatal("read-only aggregate accepted $out")
	}
	result, err = c.Mongo(ctx, Request{Table: r.Table, Action: "find", Query: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	row = result.Rows[0]
	r.Original = row.Document
	r.Token = row.Token
	r.Action = "delete"
	if _, err = c.Mongo(ctx, r); err != nil {
		t.Fatal(err)
	}
}
