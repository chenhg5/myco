package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const MaxRecordQueryLimit = 1000

var queryField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

type PageQueryInput struct {
	Fields, Sort, After string
	Where               []string
	Limit               int
	Count               bool
}
type PageQuery struct {
	Fields  []string
	Filters []RecordFilter
	Sort    []RecordSort
	Limit   int
	After   string
	Count   bool
}
type RecordFilter struct {
	Field, Op string
	Values    []RecordValue
}
type RecordSort struct {
	Field string
	Desc  bool
}

// Type is missing (empty), null, true, false, number, text, array or object.
type RecordValue struct{ Type, Text string }
type RecordQueryResult struct {
	Records []*PageRecord
	Count   *int
	Next    string
	Seq     int64
}

type RecordCursor struct {
	Scope  string `json:"q"`
	Seq    int64  `json:"s"`
	Offset int    `json:"o"`
}

func queryError(format string, a ...any) error { return Errf(CodeInvalidArgument, format, a...) }
func ParsePageQuery(in PageQueryInput) (PageQuery, error) {
	q := PageQuery{Limit: in.Limit, After: in.After, Count: in.Count}
	if in.Limit < 0 || in.Limit > MaxRecordQueryLimit {
		return q, queryError("limit must be between 0 (unlimited) and %d", MaxRecordQueryLimit)
	}
	if len(in.After) > 4096 || (in.Count && in.After != "") {
		return q, queryError("invalid cursor, or count combined with after")
	}
	fields := func(raw string, max int) ([]string, error) {
		if raw == "" {
			return nil, nil
		}
		v := strings.Split(raw, ",")
		if len(v) > max {
			return nil, queryError("too many query fields")
		}
		for _, f := range v {
			if !queryField.MatchString(f) {
				return nil, queryError("invalid query field %q: use flat letters, digits and underscores", f)
			}
		}
		return v, nil
	}
	var err error
	if q.Fields, err = fields(in.Fields, 64); err != nil {
		return q, err
	}
	if in.Sort != "" {
		ss := strings.Split(in.Sort, ",")
		if len(ss) > 8 {
			return q, queryError("at most 8 sort fields")
		}
		for _, s := range ss {
			desc := strings.HasPrefix(s, "-")
			f := strings.TrimPrefix(s, "-")
			if !queryField.MatchString(f) {
				return q, queryError("invalid sort field %q", f)
			}
			q.Sort = append(q.Sort, RecordSort{f, desc})
		}
	}
	if len(in.Where) > 32 {
		return q, queryError("at most 32 where conditions")
	}
	for _, raw := range in.Where {
		if len(raw) > 4096 {
			return q, queryError("where condition is too long")
		}
		f, rhs, ok := strings.Cut(raw, ":")
		if !ok || !queryField.MatchString(f) {
			return q, queryError("invalid where condition %q", raw)
		}
		op := "eq"
		name, arg, hasArg := strings.Cut(rhs, ":")
		aliases := map[string]string{"eq": "eq", "ne": "ne", "!=": "ne", "in": "in", "exists": "exists", "gt": "gt", ">": "gt", "lt": "lt", "<": "lt", "gte": "gte", ">=": "gte", "lte": "lte", "<=": "lte"}
		if known := aliases[name]; known != "" && (hasArg || name == "exists") {
			op = known
			rhs = arg
		}
		cond := RecordFilter{Field: f, Op: op}
		if op == "exists" {
			if rhs == "" {
				rhs = "true"
			}
			if rhs != "true" && rhs != "false" {
				return q, queryError("exists takes true or false")
			}
			cond.Values = []RecordValue{{Type: rhs}}
		} else if op == "in" {
			var values []json.RawMessage
			if strings.HasPrefix(strings.TrimSpace(rhs), "[") {
				if json.Unmarshal([]byte(rhs), &values) != nil {
					return q, queryError("in takes a JSON array or comma-separated values")
				}
			} else {
				for _, v := range strings.Split(rhs, ",") {
					values = append(values, json.RawMessage(v))
				}
			}
			if len(values) == 0 || len(values) > 100 {
				return q, queryError("in takes 1 to 100 values")
			}
			for _, v := range values {
				x, e := queryLiteral(string(v))
				if e != nil {
					return q, e
				}
				cond.Values = append(cond.Values, x)
			}
		} else {
			v, e := queryLiteral(rhs)
			if e != nil {
				return q, e
			}
			if op != "eq" && op != "ne" {
				if v.Type != "number" {
					if _, ok := RecordDate(v.Text); v.Type != "text" || !ok {
						return q, queryError("range comparisons take a number or ISO date")
					}
				}
			}
			cond.Values = []RecordValue{v}
		}
		q.Filters = append(q.Filters, cond)
	}
	return q, nil
}
func queryLiteral(s string) (RecordValue, error) {
	t := strings.TrimSpace(s)
	if json.Valid([]byte(t)) {
		v := recordValue(json.RawMessage(t))
		if v.Type == "object" || v.Type == "array" {
			return v, queryError("query values must be scalars")
		}
		if v.Type == "number" {
			n, _ := strconv.ParseFloat(v.Text, 64)
			if math.IsInf(n, 0) {
				return v, queryError("query number is out of range")
			}
		}
		return v, nil
	}
	return RecordValue{Type: "text", Text: s}, nil
}
func recordValue(raw json.RawMessage) RecordValue {
	if raw == nil {
		return RecordValue{}
	}
	t := strings.TrimSpace(string(raw))
	switch t {
	case "null":
		return RecordValue{Type: "null"}
	case "true", "false":
		return RecordValue{Type: t}
	}
	if len(t) > 0 && t[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return RecordValue{Type: "text", Text: s}
	}
	if len(t) > 0 && (t[0] == '{' || t[0] == '[') {
		var b bytes.Buffer
		_ = json.Compact(&b, raw)
		kind := "object"
		if t[0] == '[' {
			kind = "array"
		}
		return RecordValue{kind, b.String()}
	}
	return RecordValue{Type: "number", Text: t}
}
func RecordDate(s string) (int64, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.Round(time.Millisecond).UnixMilli(), true
		}
	}
	return 0, false
}
func number(v RecordValue) *big.Rat {
	if v.Type == "true" {
		return big.NewRat(1, 1)
	}
	if v.Type == "false" {
		return big.NewRat(0, 1)
	}
	if n, e := strconv.ParseInt(v.Text, 10, 64); e == nil {
		return big.NewRat(n, 1)
	}
	n, _ := strconv.ParseFloat(v.Text, 64)
	return new(big.Rat).SetFloat64(n)
}
func compareNumbers(a, b RecordValue) int {
	x, y := number(a), number(b)
	if x != nil && y != nil {
		return x.Cmp(y)
	}
	af, _ := strconv.ParseFloat(a.Text, 64)
	bf, _ := strconv.ParseFloat(b.Text, 64)
	if af < bf {
		return -1
	}
	if af > bf {
		return 1
	}
	return 0
}
func matches(v RecordValue, f RecordFilter) bool {
	if f.Op == "exists" {
		return (v.Type != "") == (f.Values[0].Type == "true")
	}
	eq := func(w RecordValue) bool {
		return v.Type == w.Type && (v.Type == "number" && compareNumbers(v, w) == 0 || v.Type != "number" && v.Text == w.Text)
	}
	switch f.Op {
	case "eq":
		return eq(f.Values[0])
	case "ne":
		return v.Type != "" && !eq(f.Values[0])
	case "in":
		for _, w := range f.Values {
			if eq(w) {
				return true
			}
		}
		return false
	}
	w := f.Values[0]
	cmp := 0
	if w.Type == "number" {
		if v.Type != "number" {
			return false
		}
		cmp = compareNumbers(v, w)
	} else {
		if v.Type != "text" {
			return false
		}
		a, ok := RecordDate(v.Text)
		if !ok {
			return false
		}
		b, _ := RecordDate(w.Text)
		if a < b {
			cmp = -1
		} else if a > b {
			cmp = 1
		}
	}
	switch f.Op {
	case "gt":
		return cmp > 0
	case "gte":
		return cmp >= 0
	case "lt":
		return cmp < 0
	case "lte":
		return cmp <= 0
	}
	return false
}
func compareValues(a, b RecordValue) int {
	rank := func(v RecordValue) int {
		switch v.Type {
		case "null":
			return 0
		case "number", "true", "false":
			return 1
		default:
			return 2
		}
	}
	x, y := rank(a), rank(b)
	if x < y {
		return -1
	}
	if x > y {
		return 1
	}
	if x == 1 {
		return compareNumbers(a, b)
	}
	return strings.Compare(a.Text, b.Text)
}
func (q PageQuery) Scope(page, collection string) string {
	b, _ := json.Marshal([]any{page, collection, q.Fields, q.Filters, q.Sort})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (q PageQuery) Offset(page, collection string, seq int64) (int, error) {
	if q.After == "" {
		return 0, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(q.After)
	if e != nil {
		return 0, queryError("invalid page cursor")
	}
	var c RecordCursor
	if json.Unmarshal(b, &c) != nil || c.Scope != q.Scope(page, collection) || c.Offset < 1 || c.Offset > MaxPageRecords || c.Seq < 0 {
		return 0, queryError("cursor belongs to another page or query, or is invalid")
	}
	if c.Seq != seq {
		return 0, Errf(CodeConflict, "page data changed; restart the query without after")
	}
	return c.Offset, nil
}
func (q PageQuery) Cursor(page, collection string, seq int64, offset int) string {
	b, _ := json.Marshal(RecordCursor{q.Scope(page, collection), seq, offset})
	return base64.RawURLEncoding.EncodeToString(b)
}
func ProjectPageRecord(r *PageRecord, fields []string) *PageRecord {
	if len(fields) == 0 {
		return r
	}
	var data map[string]json.RawMessage
	_ = json.Unmarshal([]byte(r.Data), &data)
	selected := map[string]json.RawMessage{}
	for _, f := range fields {
		if v, ok := data[f]; ok {
			selected[f] = v
		}
	}
	b, _ := json.Marshal(selected)
	out := *r
	out.Data = string(b)
	return &out
}

// ApplyPageQuery is the old-Hub fallback, with the same ordering and cursor
// contract as SQLite. It never mutates caller records or their versions.
func ApplyPageQuery(records []*PageRecord, page, collection string, seq int64, q PageQuery) (RecordQueryResult, error) {
	out := RecordQueryResult{Records: []*PageRecord{}, Seq: seq}
	offset, err := q.Offset(page, collection, seq)
	if err != nil {
		return out, err
	}
	type item struct {
		r    *PageRecord
		data map[string]json.RawMessage
	}
	items := []item{}
	for _, r := range records {
		var data map[string]json.RawMessage
		if e := json.Unmarshal([]byte(r.Data), &data); e != nil {
			return out, queryError("invalid record JSON")
		}
		yes := true
		for _, f := range q.Filters {
			if !matches(recordValue(data[f.Field]), f) {
				yes = false
				break
			}
		}
		if yes {
			items = append(items, item{r, data})
		}
	}
	if q.Count {
		n := len(items)
		out.Count = &n
		return out, nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		for _, s := range q.Sort {
			x, y := recordValue(a.data[s.Field]), recordValue(b.data[s.Field])
			if (x.Type == "") != (y.Type == "") {
				return y.Type == ""
			}
			v := compareValues(x, y)
			if v != 0 {
				if s.Desc {
					return v > 0
				}
				return v < 0
			}
		}
		if a.r.Collection != b.r.Collection {
			return a.r.Collection < b.r.Collection
		}
		if a.r.Pos != b.r.Pos {
			return a.r.Pos < b.r.Pos
		}
		return a.r.ID < b.r.ID
	})
	end := len(items)
	if q.Limit > 0 && offset+q.Limit < end {
		end = offset + q.Limit
		out.Next = q.Cursor(page, collection, seq, end)
	}
	for i := offset; i < end; i++ {
		out.Records = append(out.Records, ProjectPageRecord(items[i].r, q.Fields))
	}
	return out, nil
}

// Validate also guards direct repository callers: no SQL path is assembled
// from an unchecked field, even when a caller constructs PageQuery itself.
func (q PageQuery) Validate() error {
	if len(q.Fields) > 64 || len(q.Sort) > 8 || len(q.Filters) > 32 || q.Limit < 0 || q.Limit > MaxRecordQueryLimit || len(q.After) > 4096 || (q.Count && q.After != "") {
		return queryError("invalid query limits")
	}
	for _, f := range q.Fields {
		if !queryField.MatchString(f) {
			return queryError("invalid query field")
		}
	}
	for _, s := range q.Sort {
		if !queryField.MatchString(s.Field) {
			return queryError("invalid sort field")
		}
	}
	for _, f := range q.Filters {
		if !queryField.MatchString(f.Field) || len(f.Values) < 1 || len(f.Values) > 100 || (f.Op != "in" && len(f.Values) != 1) {
			return queryError("invalid where condition")
		}
		switch f.Op {
		case "eq", "ne", "in", "exists", "gt", "gte", "lt", "lte":
		default:
			return queryError("unknown where operator")
		}
	}
	return nil
}
