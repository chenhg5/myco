// Package requestid carries one id per request from the App through its
// backend to the hub (X-Request-Id), so a slow or failed step can be found
// in each place's log by the same id.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Header is the HTTP header the id travels in.
const Header = "X-Request-Id"

type key struct{}

// New is a fresh id: 16 hex digits.
func New() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Valid is id when it is one to pass on (1–64 of A–Z a–z 0–9 - _ .), else
// "": a caller's id never carries anything else into a log.
func Valid(id string) string {
	if id == "" || len(id) > 64 {
		return ""
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ""
		}
	}
	return id
}

// With is ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From is the id ctx carries, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}
