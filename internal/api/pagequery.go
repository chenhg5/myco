package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// PageQuery opts into records query protocol version 1. Zero/unset limit keeps
// the historical unlimited behavior; all conditions are ANDed.
type PageQuery struct {
	Fields string   `json:"fields,omitempty"`
	Where  []string `json:"where,omitempty"`
	Sort   string   `json:"sort,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	After  string   `json:"after,omitempty"`
	Count  bool     `json:"count,omitempty"`
}

func (q PageQuery) Values(collection string) url.Values {
	v := url.Values{"query_version": {"1"}}
	if collection != "" {
		v.Set("collection", collection)
	}
	if q.Fields != "" {
		v.Set("fields", q.Fields)
	}
	if q.Sort != "" {
		v.Set("sort", q.Sort)
	}
	for _, w := range q.Where {
		v.Add("where", w)
	}
	if q.Limit != 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.After != "" {
		v.Set("after", q.After)
	}
	if q.Count {
		v.Set("count", "true")
	}
	return v
}
func ParsePageQuery(v url.Values) (PageQuery, error) {
	q := PageQuery{Fields: v.Get("fields"), Where: v["where"], Sort: v.Get("sort"), After: v.Get("after")}
	var err error
	if s := v.Get("limit"); s != "" {
		q.Limit, err = strconv.Atoi(s)
		if err != nil {
			return q, fmt.Errorf("limit must be an integer")
		}
	}
	if s := v.Get("count"); s != "" {
		q.Count, err = strconv.ParseBool(s)
		if err != nil {
			return q, fmt.Errorf("count must be true or false")
		}
	}
	if s := v.Get("query_version"); s != "" && s != "1" {
		return q, fmt.Errorf("unsupported record query version")
	}
	return q, nil
}
func HasPageQuery(v url.Values) bool {
	for _, s := range []string{"query_version", "fields", "where", "sort", "limit", "after", "count"} {
		if v.Has(s) {
			return true
		}
	}
	return false
}

// PageRecordCount is a count-only response, without record bodies.
type PageRecordCount struct {
	Count        int   `json:"count"`
	Seq          int64 `json:"seq"`
	QueryVersion int   `json:"query_version"`
}
