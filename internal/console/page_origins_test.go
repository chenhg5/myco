package console

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/myco-ai/myco/internal/client"
)

func testPageOrigin(t *testing.T, s *pageServer, hub, account, id string) (string, string) {
	t.Helper()
	address, ticket, err := s.open(&Backend{}, pageScope(hub, account, id), pageKey{hub: hub, page: id, entry: "index.html"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host, ticket
}

func closeTestPageOrigins(s *pageServer) {
	for _, listener := range s.active {
		listener.close()
	}
}

func TestPageOriginIsolatesScopeAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s := loadPageOrigins(dir)
	t.Cleanup(func() { closeTestPageOrigins(s) })
	targets := [][3]string{{"one", "owner", "page"}, {"one", "owner", "other"}, {"two", "owner", "page"}, {"one", "member", "page"}}
	origins := map[string]bool{}
	var first string
	for _, k := range targets {
		address, _ := testPageOrigin(t, s, k[0], k[1], k[2])
		if origins[address] {
			t.Fatalf("reused address for a different scope: %s", address)
		}
		origins[address] = true
		if first == "" {
			first = address
		}
	}
	closeTestPageOrigins(s)
	restarted := loadPageOrigins(dir)
	t.Cleanup(func() { closeTestPageOrigins(restarted) })
	got, _ := testPageOrigin(t, restarted, "one", "owner", "page")
	if got != first {
		t.Fatalf("restart changed %s to %s", first, got)
	}
}

func TestPageOriginTakenPortNeverReassignsStorage(t *testing.T) {
	dir := t.TempDir()
	s := loadPageOrigins(dir)
	address, _ := testPageOrigin(t, s, "one", "owner", "page")
	other := loadPageOrigins(dir)
	_, _, err := other.open(&Backend{}, pageScope("one", "owner", "page"), pageKey{hub: "one", page: "page"})
	var apiErr *client.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "page_origin_unavailable" {
		t.Fatalf("expected a conflict on the original port: %v", err)
	}
	if apiErr.Status != http.StatusServiceUnavailable || apiErr.Message != "This page is open in another myco App. Quit that App, then try again." {
		t.Fatalf("unexpected page conflict guidance: %#v", apiErr)
	}
	closeTestPageOrigins(s)
	t.Cleanup(func() { closeTestPageOrigins(other) })
	if got, _ := testPageOrigin(t, other, "one", "owner", "page"); got != address {
		t.Fatalf("retry changed %s to %s", address, got)
	}
}

func TestPageOriginEvictsOnlyIdleListeners(t *testing.T) {
	s := loadPageOrigins(t.TempDir())
	t.Cleanup(func() { closeTestPageOrigins(s) })
	permanent, protected := testPageOrigin(t, s, "hub", "owner", "pinned")
	var first string
	for i := range warmPageListeners + 2 {
		id := strconv.Itoa(i)
		host, ticket := testPageOrigin(t, s, "hub", "owner", id)
		if i == 0 {
			first = host
		}
		s.release("wrong-hub", id, ticket)
		if s.tickets[ticket] == nil {
			t.Fatal("another hub could release the opening ticket")
		}
		s.release("hub", id, ticket)
	}
	if len(s.active) != warmPageListeners+1 || s.tickets[protected] == nil {
		t.Fatalf("idle limit removed a live page or left excess listeners: %d", len(s.active))
	}
	ln, err := net.Listen("tcp4", first)
	if err != nil {
		t.Fatal("old idle listener did not close:", err)
	}
	_ = ln.Close()
	if reopened, _ := testPageOrigin(t, s, "hub", "owner", "0"); reopened != first {
		t.Fatalf("idle eviction changed %s to %s", first, reopened)
	}
	resp, err := http.Get("http://" + permanent + "/not-a-page-ticket")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected response to missing ticket: %d", resp.StatusCode)
	}
}

func TestPageOriginOpenLimitKeepsLiveListeners(t *testing.T) {
	s := loadPageOrigins("")
	t.Cleanup(func() { closeTestPageOrigins(s) })
	for i := range maxPageListeners {
		testPageOrigin(t, s, "hub", "owner", strconv.Itoa(i))
	}
	_, _, err := s.open(&Backend{}, "excess", pageKey{hub: "hub", page: "other"})
	var apiErr *client.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "page_limit" || len(s.active) != maxPageListeners {
		t.Fatalf("expected the listener limit to preserve current pages: %v", err)
	}
}
