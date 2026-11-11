package console

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/zalando/go-keyring"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Cache keeps what the App's pages fetched, so the App shows it at once
// after a restart and refreshes it in the background. The web UI owns the
// entries (opaque JSON by key); here they are stored encrypted with a key
// from the system keychain, so the file alone reveals nothing. It is only a
// cache: anything wrong with it (a missing key, a damaged file, another
// version) and it starts over empty.
type Cache struct {
	db   *sql.DB
	aead cipher.AEAD
	path string
	// Max is the total size kept (bytes of encrypted data); the least
	// recently used entries go first. Default 50 MB.
	Max int64
	mu  sync.Mutex
}

// cacheVersion changes when entries written before cannot be read: the
// file is then started over.
const cacheVersion = "1"

const cacheSchema = `
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS entries (
	key  TEXT PRIMARY KEY,
	hub  TEXT NOT NULL,
	data BLOB NOT NULL,
	used INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_entries_used ON entries(used);
CREATE INDEX IF NOT EXISTS idx_entries_hub ON entries(hub);
`

// DefaultCachePath is app-cache.db next to the hub list.
func DefaultCachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "myco", "app-cache.db"), nil
}

// KeychainCacheKey returns the cache's 32-byte key from the system keychain,
// creating it the first time.
func KeychainCacheKey() ([]byte, error) {
	const user = "app-cache-key"
	if s, err := keyring.Get(keychainService, user); err == nil {
		if k, err := base64.StdEncoding.DecodeString(s); err == nil && len(k) == 32 {
			return k, nil
		}
	} else if !errors.Is(err, keyring.ErrNotFound) {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := keyring.Set(keychainService, user, base64.StdEncoding.EncodeToString(k)); err != nil {
		return nil, err
	}
	return k, nil
}

// OpenCache opens (or starts over) the cache at path with a 32-byte key.
func OpenCache(path string, key []byte) (*Cache, error) {
	if len(key) != 32 {
		return nil, errors.New("cache key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	c := &Cache{aead: aead, path: path}
	if err := c.open(); err != nil {
		// Damaged or from another version: start over.
		log.Printf("app cache: starting over: %v", err)
		c.removeFiles()
		if err := c.open(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Cache) open() error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(c.path)+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(3000)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(cacheSchema); err != nil {
		db.Close()
		return err
	}
	var v string
	err = db.QueryRow(`SELECT v FROM meta WHERE k = 'version'`).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := db.Exec(`INSERT INTO meta (k, v) VALUES ('version', ?)`, cacheVersion); err != nil {
			db.Close()
			return err
		}
	case err != nil:
		db.Close()
		return err
	case v != cacheVersion:
		db.Close()
		return fmt.Errorf("version %s, want %s", v, cacheVersion)
	}
	_ = os.Chmod(c.path, 0o600)
	c.db = db
	return nil
}

func (c *Cache) removeFiles() {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(c.path + suffix)
	}
}

func (c *Cache) max() int64 {
	if c.Max > 0 {
		return c.Max
	}
	return 50 << 20
}

// CacheEntry is one remembered answer: its key, the hub it belongs to (""
// for data of several hubs) and the JSON the UI stored.
type CacheEntry struct {
	Key  string          `json:"key"`
	Hub  string          `json:"hub"`
	Data json.RawMessage `json:"data"`
}

func (c *Cache) seal(key string, data []byte) []byte {
	nonce := make([]byte, c.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return c.aead.Seal(nonce, nonce, data, []byte(key))
}

func (c *Cache) open1(key string, blob []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(blob) < n {
		return nil, errors.New("short entry")
	}
	return c.aead.Open(nil, blob[:n], blob[n:], []byte(key))
}

// All returns the entries, most recently used first, for hubs still in the
// app (keep); entries of other hubs are deleted. One that cannot be read
// is dropped.
func (c *Cache) All(keep map[string]bool) ([]CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows, err := c.db.Query(`SELECT key, hub, data FROM entries ORDER BY used DESC`)
	if err != nil {
		return nil, err
	}
	var out []CacheEntry
	var drop []string
	for rows.Next() {
		var key, hub string
		var blob []byte
		if err := rows.Scan(&key, &hub, &blob); err != nil {
			rows.Close()
			return nil, err
		}
		if hub != "" && !keep[hub] {
			drop = append(drop, key)
			continue
		}
		data, err := c.open1(key, blob)
		if err != nil || !json.Valid(data) {
			drop = append(drop, key)
			continue
		}
		out = append(out, CacheEntry{Key: key, Hub: hub, Data: data})
	}
	rows.Close()
	for _, k := range drop {
		_, _ = c.db.Exec(`DELETE FROM entries WHERE key = ?`, k)
	}
	return out, rows.Err()
}

// Put stores entries (replacing the same keys) and deletes keys, then trims
// the cache to its size.
func (c *Cache) Put(entries []CacheEntry, del []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for i, e := range entries {
		if e.Key == "" || !json.Valid(e.Data) {
			continue
		}
		_, err := tx.Exec(`INSERT INTO entries (key, hub, data, used) VALUES (?, ?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET hub = excluded.hub, data = excluded.data, used = excluded.used`,
			e.Key, e.Hub, c.seal(e.Key, e.Data), now+int64(i))
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, k := range del {
		if _, err := tx.Exec(`DELETE FROM entries WHERE key = ?`, k); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return c.trim()
}

// trim deletes the least recently used entries beyond the size limit.
func (c *Cache) trim() error {
	var total int64
	if err := c.db.QueryRow(`SELECT COALESCE(SUM(LENGTH(data)), 0) FROM entries`).Scan(&total); err != nil {
		return err
	}
	for total > c.max() {
		var key string
		var size int64
		err := c.db.QueryRow(`SELECT key, LENGTH(data) FROM entries ORDER BY used ASC LIMIT 1`).Scan(&key, &size)
		if err != nil {
			return err
		}
		if _, err := c.db.Exec(`DELETE FROM entries WHERE key = ?`, key); err != nil {
			return err
		}
		total -= size
	}
	return nil
}

// ForgetHub deletes a hub's entries, and the entries that mix several hubs
// (they may hold some of its data).
func (c *Cache) ForgetHub(hub string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`DELETE FROM entries WHERE hub = ? OR hub = ''`, hub)
	return err
}

// Clear deletes everything.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.db.Exec(`DELETE FROM entries`); err != nil {
		return err
	}
	_, _ = c.db.Exec(`VACUUM`)
	return nil
}

// Stats is the number of entries and their size.
func (c *Cache) Stats() (n int, bytes int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err = c.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(LENGTH(data)), 0) FROM entries`).Scan(&n, &bytes)
	return n, bytes, err
}

// Close closes the file.
func (c *Cache) Close() error { return c.db.Close() }

// cacheRoutes serves the cache to the web UI: GET the entries, PUT changes,
// DELETE everything, GET /stats. Without a cache (disabled, or the keychain
// unavailable) GET answers an empty list and PUT does nothing.
func (b *Backend) cacheRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/cache", func(w http.ResponseWriter, r *http.Request) {
		out := []CacheEntry{}
		if b.Cache != nil {
			keep := map[string]bool{}
			if hubs, err := b.Store.List(); err == nil {
				for _, h := range hubs {
					keep[h.ID] = true
				}
			}
			if e, err := b.Cache.All(keep); err == nil {
				out = e
			} else {
				log.Printf("app cache: read: %v", err)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": out, "enabled": b.Cache != nil})
	})
	m.HandleFunc("PUT /app/cache", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Entries []CacheEntry `json:"entries"`
			Delete  []string     `json:"delete"`
		}
		if !decodeLimit(w, r, &in, 64<<20) {
			return
		}
		if b.Cache != nil {
			if err := b.Cache.Put(in.Entries, in.Delete); err != nil {
				log.Printf("app cache: write: %v", err) // a cache write never fails the UI
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.HandleFunc("DELETE /app/cache", func(w http.ResponseWriter, r *http.Request) {
		if b.Cache != nil {
			if err := b.Cache.Clear(); err != nil {
				writeErr(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.HandleFunc("GET /app/cache/stats", func(w http.ResponseWriter, r *http.Request) {
		var n int
		var size int64
		if b.Cache != nil {
			n, size, _ = b.Cache.Stats()
		}
		writeJSON(w, http.StatusOK, map[string]any{"enabled": b.Cache != nil, "entries": n, "bytes": size})
	})
}

// decodeLimit is decode with a larger body limit.
func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}
