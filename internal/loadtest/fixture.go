package loadtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

type identity struct {
	token string
	c     *client.Client
}

func (i *identity) connect(base string) { i.c, _ = client.New(base, i.token) }

type person struct {
	name, id string
	devices  []*identity
}
type bot struct {
	name, owner string
	user        int
	private     bool
	auth        identity
}
type fixture struct {
	people []*person
	bots   []*bot
	admin  identity
}

func (f *fixture) connect(base string) {
	f.admin.connect(base)
	for _, u := range f.people {
		for _, d := range u.devices {
			d.connect(base)
		}
	}
	for _, b := range f.bots {
		b.auth.connect(base)
		b.auth.c.Via = "runner"
	}
}
func pairingCode(link string) (string, error) {
	_, s, ok := strings.Cut(link, "/c/")
	if !ok || s == "" {
		return "", errors.New("invalid fixture pairing link")
	}
	return s, nil
}
func prepare(ctx context.Context, base, admin string, users int) (*fixture, error) {
	f := &fixture{admin: identity{token: admin}}
	f.admin.connect(base)
	a := f.admin.c
	anonymous, _ := client.New(base, "")
	if _, e := a.UpdateMember(ctx, "owner", api.MemberUpdateRequest{Name: "u001"}); e != nil {
		return nil, safeErr("owner setup", e)
	}
	if _, e := a.CreateProject(ctx, api.CreateProjectRequest{Name: "shared", Visibility: "members"}); e != nil {
		return nil, safeErr("shared project", e)
	}
	for n := 0; n < users; n++ {
		u := &person{name: fmt.Sprintf("u%03d", n+1)}
		var token string
		if n == 0 {
			r, e := a.CreateConsoleToken(ctx, "demo-device-1")
			if e != nil {
				return nil, safeErr("owner device", e)
			}
			token = r.Token
		} else {
			r, e := a.InviteMember(ctx, api.InviteRequest{Name: u.name})
			if e != nil {
				return nil, safeErr("invite member", e)
			}
			code, e := pairingCode(r.Link)
			if e != nil {
				return nil, e
			}
			paired, e := anonymous.Pair(ctx, code, "demo-device-1")
			if e != nil {
				return nil, safeErr("pair member", e)
			}
			token = paired.Token
		}
		d := &identity{token: token}
		d.connect(base)
		u.devices = append(u.devices, d)
		who, e := d.c.Whoami(ctx)
		if e != nil {
			return nil, safeErr("verify member", e)
		}
		if who.Member == nil || who.Member.Name != u.name {
			return nil, errors.New("member ownership mismatch")
		}
		u.id = who.Member.ID
		p, e := d.c.CreatePairing(ctx)
		if e != nil {
			return nil, safeErr("second pairing", e)
		}
		code, e := pairingCode(p.Link)
		if e != nil {
			return nil, e
		}
		pair, e := anonymous.Pair(ctx, code, "demo-device-2")
		if e != nil {
			return nil, safeErr("second device", e)
		}
		d2 := &identity{token: pair.Token}
		d2.connect(base)
		u.devices = append(u.devices, d2)
		f.people = append(f.people, u)
		if _, e = d.c.CreateProject(ctx, api.CreateProjectRequest{Name: u.name, Visibility: "members"}); e != nil {
			return nil, safeErr("private project", e)
		}
		for dn, dev := range u.devices {
			prefix := fmt.Sprintf("%s-d%d-", u.name, dn+1)
			enr, e := dev.c.CreateEnrollment(ctx, api.CreateEnrollmentRequest{NamePrefix: prefix, MaxUses: 5}, false)
			if e != nil {
				return nil, safeErr("enrollment", e)
			}
			ec, _ := client.New(base, enr.Token)
			for j := 0; j < 5; j++ {
				name := fmt.Sprintf("%sa%d", prefix, j+1)
				r, e := ec.Enroll(ctx, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "shell", SessionID: "demo-" + name}}, false)
				if e != nil {
					return nil, safeErr("enroll agent", e)
				}
				if r.Agent.Owner != u.name {
					return nil, errors.New("agent owner mismatch")
				}
				b := &bot{name: name, owner: u.id, user: n, private: j == 4, auth: identity{token: r.Token}}
				b.auth.connect(base)
				b.auth.c.Via = "runner"
				sum := sha256.Sum256([]byte(prefix))
				if e = b.auth.c.ReportMachine(ctx, api.MachineReport{Fingerprint: hex.EncodeToString(sum[:]), Name: prefix + "machine", OS: "linux", Arch: "amd64"}); e != nil {
					return nil, safeErr("machine report", e)
				}
				project := "shared"
				if b.private {
					project = u.name
					if _, e = dev.c.SetVisibility(ctx, name, "private"); e != nil {
						return nil, safeErr("private agent", e)
					}
				}
				if !b.private {
					if _, e = dev.c.SetVisibility(ctx, name, "team"); e != nil {
						return nil, safeErr("team agent", e)
					}
				}
				projectClient := dev.c
				if project == "shared" {
					projectClient = a
				}
				if _, e = projectClient.PutProjectMember(ctx, project, name, api.ProjectMemberRequest{}); e != nil {
					return nil, safeErr("project membership", e)
				}
				f.bots = append(f.bots, b)
			}
		}
	}
	return f, nil
}

// Historical fixtures are closed notes, written through the real repository
// while the child is stopped. No live server database is ever opened here.
func seedHistory(ctx context.Context, dir string, f *fixture, n int, seed int64) error {
	db, e := sqlite.Open(filepath.Join(dir, "hub.db"))
	if e != nil {
		return e
	}
	defer db.Close()
	// Reserve measured disk headroom before constructing a large fixture.
	res := readResources(os.Getpid(), dir, nil)
	if res.DiskFree != nil && *res.DiskFree < uint64(n)*11000+(5<<30) {
		return errors.New("insufficient disk headroom for historical fixture")
	}
	repo := sqlite.NewMessageRepo(db)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bodies := []string{strings.Repeat("demo ", 51), strings.Repeat("demo ", 819), strings.Repeat("demo ", 6553), strings.Repeat("demo ", 26214)}
	roots := map[string]string{}
	rng := rand.New(rand.NewPCG(uint64(seed), 91))
	for i := 0; i < n; i++ {
		if i%1000 == 0 {
			res := readResources(os.Getpid(), dir, nil)
			if res.DiskFree != nil && *res.DiskFree < 5<<30 {
				return errors.New("fixture disk headroom exhausted")
			}
			if e = ctx.Err(); e != nil {
				return e
			}
		}
		// Body size, parties and ownership vary independently; otherwise a
		// modulus can accidentally put every large body on just one agent.
		b := f.bots[rng.IntN(len(f.bots))]
		if i%20 == 0 {
			b = f.bots[0]
		} // a genuinely long hot node pair
		if i == 1 {
			b = f.bots[4]
		} // a private-project large-body probe
		u := f.people[b.user]
		project := "shared"
		from, fromOwner := domain.AgentName("@console:"+u.name), u.id
		if b.private {
			project = u.name
		} else if i%5 == 0 {
			peer := f.bots[((b.user+1)%len(f.people))*10+1]
			from, fromOwner = domain.AgentName(peer.name), peer.owner
		}
		id := fmt.Sprintf("msg_seed_%09d", i)
		thread := id
		if i%10 == 0 {
			key := string(from) + "/" + b.name
			if root := roots[key]; root != "" {
				thread = root
			} else {
				roots[key] = id
			}
		}
		size := 0
		switch roll := rng.IntN(20); {
		case roll == 0:
			size = 3
		case roll == 1:
			size = 2
		case roll%2 == 0:
			size = 1
		}
		if i < 4 {
			size = 3 - i
		}
		t := at.Add(time.Duration(i) * time.Millisecond)
		m := &domain.Message{ID: id, From: from, To: domain.AgentName(b.name), ThreadID: thread, Type: domain.TypeNote, Body: bodies[size], Status: domain.StatusDone, CreatedAt: t, UpdatedAt: t, FromOwner: fromOwner, Project: project}
		if e = repo.Create(ctx, m); e != nil {
			return e
		}
	}
	var check string
	if e = db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); e != nil || check != "ok" {
		return errors.New("fixture quick_check failed")
	}
	rows, e := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if e != nil {
		return e
	}
	bad := rows.Next()
	rows.Close()
	if bad {
		return errors.New("fixture foreign key check failed")
	}
	return nil
}
func probes(ctx context.Context, f *fixture) error {
	for _, u := range f.people {
		for _, d := range u.devices {
			who, e := d.c.Whoami(ctx)
			if e != nil || who.Member == nil || who.Member.ID != u.id {
				return errors.New("device ownership probe failed")
			}
		}
	}
	// A regular member must never see another member's private agent/project.
	if len(f.people) > 2 {
		other := f.people[1].devices[0].c
		private := f.bots[2*10+4]
		if _, e := other.GetAgent(ctx, private.name); !denied(e) {
			return errors.New("private agent permission probe failed")
		}
		if _, e := other.GetProject(ctx, f.people[2].name, false); !denied(e) {
			return errors.New("private project permission probe failed")
		}
	}
	return nil
}
func denied(e error) bool {
	var ce *client.Error
	return errors.As(e, &ce) && (ce.Status == 403 || ce.Status == 404)
}
func safeErr(stage string, e error) error {
	var ce *client.Error
	if errors.As(e, &ce) {
		return fmt.Errorf("%s: API %s (HTTP %d)", stage, ce.Code, ce.Status)
	}
	if errors.Is(e, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return fmt.Errorf("%s: timeout", stage)
	}
	return fmt.Errorf("%s failed", stage)
}
