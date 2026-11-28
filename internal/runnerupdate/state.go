// Package runnerupdate keeps local, credential-free upgrade receipts and the
// running runner's acknowledgement. Requests survive the App closing.
package runnerupdate

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

const IdleLimit = 6 * time.Hour
const FreshFor = 10 * time.Second

type Request = api.LocalIdleRestartRequest
type Runtime = api.LocalRunnerRuntime
type Receipt = api.CLIUpgradeReceipt

func Fresh(r *Runtime, now time.Time) bool {
	return r != nil && !r.At.After(now.Add(time.Second)) && now.Sub(r.At) <= FreshFor
}

func Path(base, kind string) string { return filepath.Join(base, "upgrade-"+kind+".json") }
func Read(base, kind string, v any) error {
	f, e := os.Open(Path(base, kind))
	if e != nil {
		return e
	}
	defer f.Close()
	return json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(v)
}
func Write(base, kind string, v any) error { return WriteFile(Path(base, kind), v) }
func WriteFile(path string, v any) error {
	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(base, ".upgrade-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func LoadRequest(base string) (*Request, error) {
	var r Request
	if e := Read(base, "request", &r); e != nil {
		return nil, e
	}
	if r.ID == "" || r.Target == "" || r.Deadline.IsZero() || r.Deadline.After(r.RequestedAt.Add(IdleLimit)) {
		return nil, errors.New("invalid idle restart request")
	}
	return &r, nil
}

var privateURLs = regexp.MustCompile(`https?://[^\s]+`)

// SafeMessage keeps an actionable reason without recording credential-bearing URLs.
func SafeMessage(s string) string { return privateURLs.ReplaceAllString(s, "[download address]") }
