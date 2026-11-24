// Package dbstudio provides bounded, context-aware database operations for the
// local desktop App. It never sends connection credentials or results to a Hub.
package dbstudio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string           { return e.Message }
func failure(code, message string) error { return &Error{Code: code, Message: message} }

type Secrets interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

// Profile is safe to return to the owning page. Passwords and full MongoDB URIs
// are stored separately in the desktop's credential store and are never read back.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Database    string `json:"database"`
	Path        string `json:"path"`
	TLS         string `json:"tls"`
	AuthSource  string `json:"auth_source"`
	ReadOnly    bool   `json:"read_only"`
	HasPassword bool   `json:"has_password"`
	HasURI      bool   `json:"has_uri"`
	Version     int    `json:"version"`
}
type ProfileInput struct {
	Profile
	Password *string `json:"password,omitempty"`
	URI      *string `json:"uri,omitempty"`
}
type credentials struct {
	Password string `json:"password"`
	URI      string `json:"uri"`
}
type Connection struct {
	Profile
	secret credentials
}

// Cell values are strings to preserve BIGINT/DECIMAL precision, binary bytes,
// nulls and dates across JavaScript. MongoDB documents use canonical Extended JSON.
type Cell struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}
type Column struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	Default   string `json:"default"`
	Primary   bool   `json:"primary"`
	Generated bool   `json:"generated"`
}
type Table struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}
type Row struct {
	Values   []Cell          `json:"values"`
	Key      map[string]Cell `json:"key,omitempty"`
	Token    string          `json:"token,omitempty"`
	Document string          `json:"document,omitempty"`
}
type Result struct {
	Columns    []Column `json:"columns"`
	Rows       []Row    `json:"rows"`
	Affected   string   `json:"affected,omitempty"`
	Truncated  bool     `json:"truncated"`
	ReadOnly   bool     `json:"read_only"`
	DurationMS int64    `json:"duration_ms"`
}
type Request struct {
	Connection string          `json:"connection"`
	Database   string          `json:"database"`
	Schema     string          `json:"schema"`
	Table      string          `json:"table"`
	Action     string          `json:"action"`
	SQL        string          `json:"sql"`
	Mode       string          `json:"mode"`
	RequestID  string          `json:"request_id"`
	TimeoutMS  int             `json:"timeout_ms"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
	OrderBy    string          `json:"order_by"`
	Descending bool            `json:"descending"`
	Filter     string          `json:"filter"`
	Key        map[string]Cell `json:"key"`
	Token      string          `json:"token"`
	Values     map[string]Cell `json:"values"`
	Query      string          `json:"query"`
	Document   string          `json:"document"`
	Original   string          `json:"original"`
}

const maxResult = 4 << 20
const maxCell = 512 << 10

func limit(n int) int {
	if n < 1 {
		return 100
	}
	if n > 1000 {
		return 1000
	}
	return n
}
func rowToken(columns []Column, values []Cell) string {
	b, _ := json.Marshal(struct {
		Columns []Column
		Values  []Cell
	}{columns, values})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ident(s, engine string) (string, error) {
	if s == "" || len(s) > 256 {
		return "", failure("invalid_argument", "a valid table or column name is required")
	}
	for _, c := range s {
		if c == 0 {
			return "", failure("invalid_argument", "invalid identifier")
		}
	}
	quote := byte('"')
	if engine == "mysql" {
		quote = '`'
	}
	q := string(quote)
	return q + strings.ReplaceAll(s, q, q+q) + q, nil
}
func tableIdent(r Request, engine string) (string, error) {
	t, err := ident(r.Table, engine)
	if err != nil {
		return "", err
	}
	if r.Schema == "" || engine == "sqlite" {
		return t, nil
	}
	s, err := ident(r.Schema, engine)
	return s + "." + t, err
}
func placeholder(engine string, n int) string {
	if engine == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
