// Package uispec is the contract for myco-ui blocks: structured UI an agent
// puts in a message body as a fenced code block (info string "myco-ui")
// holding JSON. The App draws it with a fixed set of components; this
// package parses, normalizes and validates such a block against the same
// catalog the App uses (catalog.json, copied to web/src/lib/ui-catalog.json).
//
// It is a leaf package: it imports nothing else from myco.
package uispec

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed catalog.json
var catalogJSON []byte

// Limits bound the size of one block.
type Limits struct {
	MaxBytes    int `json:"max_bytes"`
	MaxElements int `json:"max_elements"`
	MaxDepth    int `json:"max_depth"`
	MaxList     int `json:"max_list"`
}

// Prop describes one prop of a component, or one field of a list entry.
type Prop struct {
	Type        string          `json:"type"`
	Required    bool            `json:"required,omitempty"`
	Values      []string        `json:"values,omitempty"`
	Min         *float64        `json:"min,omitempty"`
	Max         *float64        `json:"max,omitempty"`
	Of          string          `json:"of,omitempty"`
	Items       map[string]Prop `json:"items,omitempty"`
	Description string          `json:"description,omitempty"`

	// ItemOrder is the order of Items as written in the catalog.
	ItemOrder []string `json:"-"`
}

// Component is one drawable component.
type Component struct {
	Group       string          `json:"group"`
	Description string          `json:"description"`
	Children    bool            `json:"children,omitempty"`
	Props       map[string]Prop `json:"props"`

	// PropOrder is the order of Props as written in the catalog.
	PropOrder []string `json:"-"`
}

// CatalogSpec is the parsed catalog.
type CatalogSpec struct {
	Version    int                  `json:"version"`
	Fence      string               `json:"fence"`
	About      string               `json:"about"`
	Limits     Limits               `json:"limits"`
	Tones      []string             `json:"tones"`
	Types      map[string]string    `json:"types"`
	Components map[string]Component `json:"components"`
	Common     map[string]Prop      `json:"common"`
	Example    json.RawMessage      `json:"example"`

	// ComponentOrder and TypeOrder keep the catalog's order.
	ComponentOrder []string `json:"-"`
	TypeOrder      []string `json:"-"`
	CommonOrder    []string `json:"-"`
}

var catalog = mustLoad(catalogJSON)

// Catalog returns the embedded catalog. Callers must not modify it.
func Catalog() *CatalogSpec { return catalog }

// CatalogJSON returns the catalog file as embedded.
func CatalogJSON() []byte { return bytes.Clone(catalogJSON) }

func mustLoad(b []byte) *CatalogSpec {
	c, err := loadCatalog(b)
	if err != nil {
		panic("uispec: bad catalog.json: " + err.Error())
	}
	return c
}

func loadCatalog(b []byte) (*CatalogSpec, error) {
	var c CatalogSpec
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	// Key order, for the guide and for stable problem lists.
	var raw struct {
		Types      json.RawMessage `json:"types"`
		Components json.RawMessage `json:"components"`
		Common     json.RawMessage `json:"common"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var err error
	if c.TypeOrder, err = objectKeys(raw.Types); err != nil {
		return nil, err
	}
	if c.ComponentOrder, err = objectKeys(raw.Components); err != nil {
		return nil, err
	}
	if c.CommonOrder, err = objectKeys(raw.Common); err != nil {
		return nil, err
	}
	var comps map[string]struct {
		Props json.RawMessage `json:"props"`
	}
	if err := json.Unmarshal(raw.Components, &comps); err != nil {
		return nil, err
	}
	for name, comp := range c.Components {
		comp.PropOrder, err = objectKeys(comps[name].Props)
		if err != nil {
			return nil, err
		}
		var props map[string]json.RawMessage
		if len(comps[name].Props) > 0 {
			if err := json.Unmarshal(comps[name].Props, &props); err != nil {
				return nil, err
			}
		}
		for pn, p := range comp.Props {
			if err := fillItemOrder(&p, props[pn]); err != nil {
				return nil, err
			}
			comp.Props[pn] = p
		}
		c.Components[name] = comp
	}
	for _, comp := range c.Components {
		for pn, p := range comp.Props {
			if _, ok := c.Types[p.Type]; !ok {
				return nil, fmt.Errorf("prop %s has unknown type %q", pn, p.Type)
			}
		}
	}
	return &c, nil
}

func fillItemOrder(p *Prop, raw json.RawMessage) error {
	if len(p.Items) == 0 || len(raw) == 0 {
		return nil
	}
	var r struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	order, err := objectKeys(r.Items)
	if err != nil {
		return err
	}
	p.ItemOrder = order
	var items map[string]json.RawMessage
	if err := json.Unmarshal(r.Items, &items); err != nil {
		return err
	}
	for k, sub := range p.Items {
		if err := fillItemOrder(&sub, items[k]); err != nil {
			return err
		}
		p.Items[k] = sub
	}
	return nil
}

// objectKeys returns the keys of a JSON object in document order.
func objectKeys(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}
