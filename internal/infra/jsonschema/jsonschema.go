// Package jsonschema checks workflow contracts completely: JSON Schema
// draft 2020-12 (oneOf, $ref, pattern, minimum, …), the same validator
// agentflow uses.
package jsonschema

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/myco-ai/myco/internal/domain"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Contracts implements domain.Contracts.
type Contracts struct {
	mu    sync.Mutex
	cache map[string]*js.Schema
}

var _ domain.Contracts = (*Contracts)(nil)

// New returns a validator that keeps compiled schemas.
func New() *Contracts { return &Contracts{cache: map[string]*js.Schema{}} }

func key(def map[string]any) string { return fmt.Sprintf("%v", def) }

func (c *Contracts) compile(def map[string]any) (*js.Schema, error) {
	k := key(def)
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.cache[k]; ok {
		return s, nil
	}
	comp := js.NewCompiler()
	comp.DefaultDraft(js.Draft2020)
	const loc = "mem://myco/contract.json"
	if err := comp.AddResource(loc, def); err != nil {
		return nil, err
	}
	s, err := comp.Compile(loc)
	if err != nil {
		return nil, err
	}
	if len(c.cache) > 500 {
		c.cache = map[string]*js.Schema{}
	}
	c.cache[k] = s
	return s, nil
}

// Compile reports whether def is a valid schema.
func (c *Contracts) Compile(def map[string]any) error {
	if len(def) == 0 {
		return nil
	}
	_, err := c.compile(def)
	if err != nil {
		return fmt.Errorf("%s", firstLine(err.Error()))
	}
	return nil
}

// Validate lists how v does not fit def: one line per failing place
// ("/approved: missing property", …), sorted.
func (c *Contracts) Validate(def map[string]any, v any) []string {
	if len(def) == 0 {
		return nil
	}
	s, err := c.compile(def)
	if err != nil {
		return []string{"the schema is not valid: " + firstLine(err.Error())}
	}
	err = s.Validate(v)
	if err == nil {
		return nil
	}
	ve, ok := err.(*js.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	collect(ve, &out)
	sort.Strings(out)
	if len(out) == 0 {
		out = []string{firstLine(ve.Error())}
	}
	return out
}

func collect(e *js.ValidationError, out *[]string) {
	if len(e.Causes) == 0 {
		path := "/" + strings.Join(e.InstanceLocation, "/")
		msg := e.ErrorKind.LocalizedString(printer)
		*out = append(*out, path+": "+msg)
		return
	}
	for _, c := range e.Causes {
		collect(c, out)
	}
}

var printer = message.NewPrinter(language.English)

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
