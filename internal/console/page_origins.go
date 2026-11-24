package console

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

const (
	maxPageListeners  = 128
	warmPageListeners = 32
)

type pageKey struct {
	hub, page, entry string
	version          int
	client           *client.Client
	credential       [32]byte
}

type pageListener struct {
	port   int
	socket net.Listener
	server *http.Server
	keys   map[string]pageKey
	idle   uint64
	mu     sync.RWMutex
}

func (s *pageListener) close() {
	_ = s.socket.Close()
	_ = s.server.Close()
}

func (s *pageListener) get(ticket string) (pageKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[ticket]
	return k, ok
}

type pageServer struct {
	mu             sync.Mutex
	path           string
	legacyPort     int
	ports          map[string]int
	reserved       map[int]bool
	active         map[string]*pageListener
	tickets        map[string]*pageListener
	sequence       uint64
	live           map[string]int
	recovery       *http.Server
	recoveryTicket string
	recoveryExpiry time.Time
	err            error
}

func loadPageOrigins(dir string) *pageServer {
	s := &pageServer{
		ports: make(map[string]int), reserved: make(map[int]bool),
		active: make(map[string]*pageListener), tickets: make(map[string]*pageListener),
	}
	if dir == "" {
		return s
	}
	s.path = filepath.Join(dir, "page-origins.json")
	if old, err := os.ReadFile(filepath.Join(dir, "page-port")); err == nil {
		port, err := strconv.Atoi(strings.TrimSpace(string(old)))
		if err == nil && port > 1024 && port <= 65535 {
			s.legacyPort = port
			s.reserved[port] = true
		}
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		s.err = err
		return s
	}
	if err := json.Unmarshal(raw, &s.ports); err != nil || s.ports == nil {
		s.err = errors.New("the page origin registry cannot be read")
		return s
	}
	for _, port := range s.ports {
		if port <= 1024 || port > 65535 || s.reserved[port] {
			s.err = errors.New("the page origin registry contains an invalid or duplicate port")
			return s
		}
		s.reserved[port] = true
	}
	return s
}

func (b *Backend) pageServer() (*pageServer, error) {
	b.pagesOnce.Do(func() {
		dir := ""
		if b.Store != nil && b.Store.path != "" {
			dir = filepath.Dir(b.Store.path)
		}
		b.pages = loadPageOrigins(dir)
	})
	return b.pages, b.pages.err
}

func (s *pageServer) save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".page-origins-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = json.NewEncoder(tmp).Encode(s.ports)
	if err == nil {
		err = tmp.Sync()
	}
	if err := errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func pageScope(hub, account, page string) string {
	raw, _ := json.Marshal([3]string{hub, account, page})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Keep collisions open during selection so the kernel does not hand the
// same reserved port straight back. Never assign an origin to a different page.
func (s *pageServer) listen() (net.Listener, error) {
	var collisions []net.Listener
	defer func() {
		for _, ln := range collisions {
			_ = ln.Close()
		}
	}()
	for range 64 {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		if !s.reserved[ln.Addr().(*net.TCPAddr).Port] {
			return ln, nil
		}
		collisions = append(collisions, ln)
	}
	return nil, errors.New("no unused local page port found")
}

func (s *pageServer) evictIdle() bool {
	var oldest string
	for scope, page := range s.active {
		if page.idle != 0 && (oldest == "" || page.idle < s.active[oldest].idle) {
			oldest = scope
		}
	}
	if oldest == "" {
		return false
	}
	s.active[oldest].close()
	delete(s.active, oldest)
	return true
}

func (s *pageServer) open(b *Backend, scope string, key pageKey) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var random [20]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", "", err
	}
	ticket := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random[:]))
	page := s.active[scope]
	if page == nil {
		// Another App process sharing this profile may have reserved ports
		// since we loaded it. Serialize reading, allocation and replacement.
		if s.path != "" {
			if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
				return "", "", err
			}
			unlock, err := lockPageOrigins(s.path + ".lock")
			if err != nil {
				return "", "", &client.Error{Code: "page_origin_busy", Status: http.StatusServiceUnavailable, Message: "the local page origin registry is busy; retry opening the page"}
			}
			defer unlock()
			state := loadPageOrigins(filepath.Dir(s.path))
			if state.err != nil {
				return "", "", state.err
			}
			s.ports, s.reserved, s.legacyPort = state.ports, state.reserved, state.legacyPort
		}
		if len(s.active) >= maxPageListeners && !s.evictIdle() {
			return "", "", &client.Error{Code: "page_limit", Status: http.StatusServiceUnavailable, Message: "too many open pages; close a page and retry"}
		}
		port := s.ports[scope]
		var ln net.Listener
		var err error
		if port == 0 {
			ln, err = s.listen()
		} else {
			ln, err = net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
		}
		if err != nil {
			return "", "", &client.Error{Code: "page_origin_unavailable", Status: http.StatusServiceUnavailable, Message: "the page's local storage address is unavailable; close the other app using it and retry"}
		}
		if port == 0 {
			port = ln.Addr().(*net.TCPAddr).Port
			s.ports[scope] = port
			if err := s.save(); err != nil {
				delete(s.ports, scope)
				_ = ln.Close()
				return "", "", err
			}
			s.reserved[port] = true
		}
		page = &pageListener{port: port, socket: ln, keys: make(map[string]pageKey)}
		page.server = &http.Server{Handler: b.pageHandler(page), ReadHeaderTimeout: 10 * time.Second}
		s.active[scope] = page
		go func() { _ = page.server.Serve(ln) }()
	}
	page.idle = 0
	page.mu.Lock()
	page.keys[ticket] = key
	page.mu.Unlock()
	s.tickets[ticket] = page
	return fmt.Sprintf("http://127.0.0.1:%d/p/%s/%s", page.port, ticket, escPath(key.entry)), ticket, nil
}

func (s *pageServer) release(hub, id, ticket string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := s.tickets[ticket]
	if page == nil {
		return
	}
	page.mu.Lock()
	key := page.keys[ticket]
	if key.hub != hub || key.page != id {
		page.mu.Unlock()
		return
	}
	delete(page.keys, ticket)
	count := len(page.keys)
	page.mu.Unlock()
	delete(s.tickets, ticket)
	if count != 0 {
		return
	}
	s.sequence++
	page.idle = s.sequence
	warm := 0
	for _, page := range s.active {
		if page.idle != 0 {
			warm++
		}
	}
	if warm > warmPageListeners {
		s.evictIdle()
	}
}
