package console

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// What this app uploaded is kept here too: a picture you sent shows in your
// chat at once (as a thumbnail, full size) without asking the hub for it,
// on hubs too old to give files back and after a restart. Each file is
// sealed with the cache's key (the folder alone reveals nothing), the
// oldest go first past sentFilesMax, and none is kept past sentFilesFor.

const (
	sentFilesMax = 300 << 20
	sentFilesFor = 14 * 24 * time.Hour
)

type sentMeta struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
}

func (c *Cache) sentDir() string { return filepath.Join(filepath.Dir(c.path), "sent-files") }

func sentKey(hub, id string) string { return "sent\x00" + hub + "\x00" + id }

func (c *Cache) sentPath(hub, id string) string {
	h := sha256.Sum256([]byte(sentKey(hub, id)))
	return filepath.Join(c.sentDir(), hex.EncodeToString(h[:16]))
}

// PutSent keeps a copy of a file this app uploaded to hub as id.
func (c *Cache) PutSent(hub, id, name, contentType string, data []byte) error {
	if c == nil || id == "" {
		return nil
	}
	meta, _ := json.Marshal(sentMeta{Name: name, ContentType: contentType})
	plain := make([]byte, 0, 4+len(meta)+len(data))
	plain = append(plain, byte(len(meta)>>24), byte(len(meta)>>16), byte(len(meta)>>8), byte(len(meta)))
	plain = append(plain, meta...)
	plain = append(plain, data...)
	if err := os.MkdirAll(c.sentDir(), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.sentDir(), ".put-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(c.seal(sentKey(hub, id), plain))
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), c.sentPath(hub, id)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	c.trimSent()
	return nil
}

// Sent returns the copy of a file this app uploaded, if it still has it.
func (c *Cache) Sent(hub, id string) (name, contentType string, data []byte, ok bool) {
	if c == nil || id == "" {
		return "", "", nil, false
	}
	p := c.sentPath(hub, id)
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > sentFilesFor {
		return "", "", nil, false
	}
	blob, err := os.ReadFile(p)
	if err != nil {
		return "", "", nil, false
	}
	plain, err := c.open1(sentKey(hub, id), blob)
	if err != nil || len(plain) < 4 {
		_ = os.Remove(p)
		return "", "", nil, false
	}
	n := int(plain[0])<<24 | int(plain[1])<<16 | int(plain[2])<<8 | int(plain[3])
	if n < 0 || 4+n > len(plain) {
		return "", "", nil, false
	}
	var m sentMeta
	if json.Unmarshal(plain[4:4+n], &m) != nil {
		return "", "", nil, false
	}
	return m.Name, m.ContentType, plain[4+n:], true
}

// trimSent drops copies past their time, then the oldest past the budget.
func (c *Cache) trimSent() {
	entries, err := os.ReadDir(c.sentDir())
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		at   time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		p := filepath.Join(c.sentDir(), e.Name())
		if time.Since(info.ModTime()) > sentFilesFor {
			_ = os.Remove(p)
			continue
		}
		files = append(files, file{p, info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for _, f := range files {
		if total <= sentFilesMax {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
