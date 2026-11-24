package console_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/console"
	"github.com/chenhg5/myco/internal/dbstudio"
)

func TestDatabaseNativeBridgeAndProductionUI(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for browser acceptance")
	}
	home := t.TempDir()
	file := filepath.Join(home, "studio.db")
	db, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE products(id INTEGER PRIMARY KEY,name TEXT NOT NULL,price DECIMAL(12,2),note TEXT);INSERT INTO products VALUES(9007199254740993,'Real native row',99.9,NULL)"); err != nil {
		t.Fatal(err)
	}
	secrets := &memSecrets{m: map[string]string{}}
	store := dbstudio.Store{Home: home, Scope: "https://workspace.example\x00pg_native", Secrets: secrets}
	p, err := store.Save(dbstudio.ProfileInput{Profile: dbstudio.Profile{Name: "Native SQLite", Engine: "sqlite", Path: file, ReadOnly: false}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&console.Backend{Local: &console.Local{Home: home}, Secrets: secrets}).Handler())
	defer server.Close()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	osAllow(t, server.URL, "https://workspace.example", "pg_native")
	input, _ := json.Marshal(map[string]string{"url": server.URL, "connection": p.ID})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(root, "templates", "database.integration.mjs"))
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native database browser: %v\n%s", err, out)
	}
	if strings.HasPrefix(string(out), "SKIP ") {
		t.Skip(strings.TrimSpace(string(out)))
	}
	t.Log(string(out))
}
