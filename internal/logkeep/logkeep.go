// Package logkeep keeps a service's log file from growing without end.
//
// A hub or runner installed as a service writes its log to a file the
// service manager opened (launchd, systemd's append:, or --log-file on
// Windows) and nothing ever trimmed it. Keep copies the file aside once a
// day or when it gets big (name.YYYYMMDD, or name.YYYYMMDD-HHMMSS when one
// day needs several), empties it (writers opened for appending go on at
// its new end; a line written in between may be lost, never the file), and
// deletes copies older than the days kept.
package logkeep

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Policy is how much log to keep.
type Policy struct {
	// Days of copies to keep (default 14).
	Days int
	// MaxBytes: a file bigger than this is copied aside at once (default 20 MB).
	MaxBytes int64
	// Every: how often to look (default 10 minutes).
	Every time.Duration
}

func (p Policy) withDefaults() Policy {
	if p.Days <= 0 {
		p.Days = 14
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = 20 << 20
	}
	if p.Every <= 0 {
		p.Every = 10 * time.Minute
	}
	return p
}

// IsStderr says whether this process's standard error is the file at path
// (a service writing its log there), so only that file is ever trimmed.
func IsStderr(path string) bool {
	a, err := os.Stderr.Stat()
	if err != nil || !a.Mode().IsRegular() {
		return false
	}
	b, err := os.Stat(path)
	return err == nil && os.SameFile(a, b)
}

// Run looks after path until done is closed: at once, then every p.Every.
func Run(path string, p Policy, done <-chan struct{}) {
	p = p.withDefaults()
	t := time.NewTicker(p.Every)
	defer t.Stop()
	for {
		_ = Once(path, p, time.Now())
		select {
		case <-done:
			return
		case <-t.C:
		}
	}
}

// Once copies path aside when it was last written on an earlier day than
// now or is bigger than p.MaxBytes, empties it, and deletes copies older
// than p.Days.
func Once(path string, p Policy, now time.Time) error {
	p = p.withDefaults()
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	day := func(t time.Time) string { return t.Format("20060102") }
	if st.Size() > 0 && (st.Size() > p.MaxBytes || day(st.ModTime()) != day(now)) {
		// (named for the day it holds: the day it was last written)
		name := path + "." + day(st.ModTime())
		if _, err := os.Stat(name); err == nil {
			name = path + "." + st.ModTime().Format("20060102-150405")
		}
		if err := copyFile(path, name); err != nil {
			return err
		}
		if err := os.Truncate(path, 0); err != nil {
			return err
		}
	}
	return prune(path, p.Days, now)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}

// prune deletes path's copies (path.YYYYMMDD…) older than days.
func prune(path string, days int, now time.Time) error {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cut := now.AddDate(0, 0, -days).Format("20060102")
	var old []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, base+".") {
			continue
		}
		stamp := strings.TrimPrefix(n, base+".")
		if len(stamp) < 8 || !allDigits(stamp[:8]) {
			continue
		}
		if stamp[:8] < cut {
			old = append(old, filepath.Join(dir, n))
		}
	}
	sort.Strings(old)
	for _, f := range old {
		_ = os.Remove(f)
	}
	return nil
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
