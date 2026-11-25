//go:build !windows

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const groupRecordName = ".myco-process-group.json"

type groupProcess struct {
	PID    int    `json:"pid"`
	PGID   int    `json:"pgid"`
	Start  string `json:"start"`
	Zombie bool   `json:"-"`
}
type browserGroup struct {
	Version int            `json:"version"`
	Nonce   string         `json:"nonce"`
	PGID    int            `json:"pgid"`
	Owner   groupProcess   `json:"owner"`
	Members []groupProcess `json:"members"`
}
type browserLaunch struct {
	Binary     string       `json:"binary"`
	Args       []string     `json:"args"`
	ProfileDir string       `json:"profile_dir"`
	Owner      groupProcess `json:"owner"`
}
type groupCleanup struct {
	DataDir  string        `json:"data_dir"`
	Owner    *groupProcess `json:"owner,omitempty"`
	OwnerPID int           `json:"owner_pid,omitempty"`
}
type profileProcessError struct{ Reason string }

func (e *profileProcessError) Error() string { return "profile_locked: " + e.Reason }
func groupError(reason string) error         { return &profileProcessError{Reason: reason} }
func ownProcess() (groupProcess, error) {
	all, err := groupProcesses(0)
	if err != nil {
		return groupProcess{}, err
	}
	for _, p := range all {
		if p.PID == os.Getpid() {
			return p, nil
		}
	}
	return groupProcess{}, errors.New("process identity unavailable")
}
func sameProcess(a, b groupProcess) bool {
	return a.PID == b.PID && a.Start != "" && a.Start == b.Start
}
func validProfileDir(dir string) error {
	if !filepath.IsAbs(dir) || !idPattern.MatchString(filepath.Base(dir)) || filepath.Base(filepath.Dir(dir)) != "profiles" {
		return errors.New("invalid profile directory")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || canonical != dir {
		return errors.New("invalid profile directory")
	}
	return nil
}
func saveBrowserGroup(dir string, r browserGroup) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	name := filepath.Join(dir, groupRecordName+".tmp-"+rand.Text())
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(name)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, groupRecordName)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func readBrowserGroup(dir string) (browserGroup, error) {
	var r browserGroup
	path := filepath.Join(dir, groupRecordName)
	info, err := os.Lstat(path)
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		return r, groupError("leftover_unverified")
	}
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 64<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil || r.Version != 1 || len(r.Nonce) < 20 || r.PGID <= 1 || r.PGID == syscall.Getpgrp() || len(r.Members) == 0 || len(r.Members) > 512 {
		return r, groupError("leftover_unverified")
	}
	for _, p := range r.Members {
		if p.PID <= 1 || p.PGID != r.PGID || p.Start == "" {
			return r, groupError("leftover_unverified")
		}
	}
	return r, nil
}
func groupGone(pgid int) (bool, error) {
	err := syscall.Kill(-pgid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, err
}
func verifiedGroup(dir string, r browserGroup, all []groupProcess) bool {
	for _, p := range all {
		for _, old := range r.Members {
			if sameProcess(p, old) {
				return true
			}
		}
	}
	// A child created after the last snapshot can still prove profile ownership.
	for _, p := range all {
		if p.Zombie {
			continue
		}
		args, err := processArguments(p.PID)
		if err != nil {
			continue
		}
		for _, arg := range args {
			if arg == "--user-data-dir="+dir {
				return true
			}
		}
	}
	return false
}
func snapshotGroup(dir string, r *browserGroup) error {
	all, err := groupProcesses(r.PGID)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		gone, e := groupGone(r.PGID)
		if e != nil {
			return e
		}
		if gone {
			return nil
		}
	}
	if !verifiedGroup(dir, *r, all) {
		return groupError("leftover_unverified")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].PID < all[j].PID })
	if len(all) > 512 {
		return groupError("leftover_unverified")
	}
	previous, _ := json.Marshal(r.Members)
	next, _ := json.Marshal(all)
	if string(previous) == string(next) {
		return nil
	}
	nextRecord := *r
	nextRecord.Members = all
	if err := saveBrowserGroup(dir, nextRecord); err != nil {
		return err
	}
	*r = nextRecord
	return nil
}

// The caller holds the profile lease. Never remove a record or release the
// lease until the kernel confirms that the recorded group no longer exists.
func terminateBrowserGroup(dir string, r browserGroup, reapLeader bool) error {
	for _, phase := range []struct {
		signal syscall.Signal
		wait   time.Duration
	}{{syscall.SIGTERM, 5 * time.Second}, {syscall.SIGKILL, 5 * time.Second}} {
		gone, err := groupGone(r.PGID)
		if err != nil {
			return groupError("cleanup_failed")
		}
		if gone {
			return removeBrowserGroup(dir)
		}
		if err = snapshotGroup(dir, &r); err != nil {
			var pe *profileProcessError
			if errors.As(err, &pe) {
				return err
			}
			return groupError("cleanup_failed")
		}
		if err = syscall.Kill(-r.PGID, phase.signal); err != nil && !errors.Is(err, syscall.ESRCH) {
			return groupError("cleanup_failed")
		}
		until := time.Now().Add(phase.wait)
		for time.Now().Before(until) {
			// Linux subreaper adopts orphaned Chrome helpers. Cmd.Wait owns the leader.
			for _, p := range r.Members {
				if reapLeader || p.PID != r.PGID {
					reapBrowserChild(p.PID)
				}
			}
			gone, err = groupGone(r.PGID)
			if err != nil {
				return groupError("cleanup_failed")
			}
			if gone {
				return removeBrowserGroup(dir)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	return groupError("cleanup_pending")
}
func recoverProfileProcesses(dir string, owner *groupProcess) error {
	r, err := readBrowserGroup(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return groupError("leftover_unverified")
	}
	if owner != nil && !sameProcess(r.Owner, *owner) {
		return nil
	}
	if r.Owner.PID != os.Getpid() {
		all, identityErr := groupProcesses(0)
		if identityErr != nil {
			return groupError("leftover_unverified")
		}
		for _, p := range all {
			if sameProcess(p, r.Owner) {
				return groupError("leftover_unverified")
			}
		}
	}

	return terminateBrowserGroup(dir, r, true)
}
func recoverBrowserGroups(in groupCleanup) error {
	if !filepath.IsAbs(in.DataDir) {
		return errors.New("invalid private configuration")
	}
	entries, err := os.ReadDir(filepath.Join(in.DataDir, "profiles"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var result error
	for _, entry := range entries {
		if !entry.IsDir() || !idPattern.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(in.DataDir, "profiles", entry.Name())
		if validProfileDir(dir) != nil {
			continue
		}
		lease, e := lockProfile(filepath.Join(dir, ".myco-lease"))
		// A just-orphaned launch shim still holds the inherited lease until
		// its durable record and parent check finish. Wait only for a dead,
		// matching owner; a live component's profile is never disturbed.
		deadline := time.Now().Add(5 * time.Second)
		for e != nil && time.Now().Before(deadline) {
			record, readErr := readBrowserGroup(dir)
			if readErr != nil || (in.OwnerPID != 0 && record.Owner.PID != in.OwnerPID) {
				break
			}
			alive := false
			all, identityErr := groupProcesses(0)
			if identityErr != nil {
				break
			}
			for _, p := range all {
				if sameProcess(p, record.Owner) {
					alive = true
					break
				}
			}
			if alive {
				break
			}
			time.Sleep(25 * time.Millisecond)
			lease, e = lockProfile(filepath.Join(dir, ".myco-lease"))
		}
		if e != nil {
			continue
		}
		if in.OwnerPID != 0 {
			r, readErr := readBrowserGroup(dir)
			if readErr == nil && r.Owner.PID != in.OwnerPID {
				lease.Close()
				continue
			}
		}
		e = recoverProfileProcesses(dir, in.Owner)
		if e != nil {
			result = e
		}
		lease.Close()
	}
	return result
}
func newBrowserCommand(binary string, args []string, dir string, lease *os.File) (*exec.Cmd, error) {
	owner, err := ownProcess()
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cfg, err := json.Marshal(browserLaunch{Binary: binary, Args: args, ProfileDir: dir, Owner: owner})
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "--launch-browser")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.ExtraFiles = []*os.File{lease}
	cmd.Stdin = strings.NewReader(string(cfg))
	return cmd, nil
}
func launchBrowser(in browserLaunch) error {
	allowed := false
	for _, binary := range installedBrowsers() {
		if binary == in.Binary {
			allowed = true
		}
	}
	if !allowed || validProfileDir(in.ProfileDir) != nil || len(in.Args) > 128 {
		return errors.New("invalid private browser configuration")
	}
	found := false
	for _, arg := range in.Args {
		if strings.HasPrefix(arg, "--user-data-dir=") {
			if arg != "--user-data-dir="+in.ProfileDir {
				return errors.New("invalid profile argument")
			}
			found = true
		}
	}
	if !found {
		return errors.New("missing profile argument")
	}
	// The inherited lease closes the parent-death/write race: a replacement
	// component cannot launch this profile while an old shim is still recording.
	lease := os.NewFile(3, "profile-lease")
	if lease == nil {
		return errors.New("missing inherited profile lease")
	}
	defer lease.Close()
	actual, leaseErr := lease.Stat()
	expected, pathErr := os.Stat(filepath.Join(in.ProfileDir, ".myco-lease"))
	if leaseErr != nil || pathErr != nil || !os.SameFile(actual, expected) {
		return errors.New("invalid inherited profile lease")
	}
	syscall.CloseOnExec(3) // Chrome must not retain the component's flock.
	me, err := ownProcess()
	if err != nil {
		return err
	}
	if me.PGID != me.PID || me.PID <= 1 {
		return errors.New("browser group unavailable")
	}
	parentAlive := false
	all, err := groupProcesses(0)
	if err != nil {
		return err
	}
	for _, p := range all {
		if sameProcess(p, in.Owner) && p.PID == os.Getppid() {
			parentAlive = true
		}
	}
	if !parentAlive {
		return errors.New("browser owner exited")
	}
	r := browserGroup{Version: 1, Nonce: rand.Text(), PGID: me.PID, Owner: in.Owner, Members: []groupProcess{me}}
	if err = saveBrowserGroup(in.ProfileDir, r); err != nil {
		return err
	}
	parentAlive = false
	all, err = groupProcesses(0)
	if err != nil {
		return err
	}
	for _, p := range all {
		if sameProcess(p, in.Owner) && p.PID == os.Getppid() {
			parentAlive = true
		}
	}
	if !parentAlive {
		return errors.New("browser owner exited")
	}
	// Record durability precedes exec: a component crash cannot create an
	// unrecorded Chrome group. The executable and profile are selected locally.
	return syscall.Exec(in.Binary, append([]string{in.Binary}, in.Args...), os.Environ())
}
func browserInternalMode(args []string, input io.Reader) (bool, error) {
	if len(args) != 2 || (args[1] != "--launch-browser" && args[1] != "--cleanup-browser-groups") {
		return false, nil
	}
	dec := json.NewDecoder(io.LimitReader(input, 128<<10))
	dec.DisallowUnknownFields()
	if args[1] == "--launch-browser" {
		var in browserLaunch
		if dec.Decode(&in) != nil {
			return true, errors.New("invalid private configuration")
		}
		return true, launchBrowser(in)
	}
	var in groupCleanup
	if dec.Decode(&in) != nil {
		return true, errors.New("invalid private configuration")
	}
	if err := adoptBrowserChildren(); err != nil {
		return true, err
	}
	return true, recoverBrowserGroups(in)
}
func guardBrowser(cmd *exec.Cmd, dir string) (func() error, error) {
	var r browserGroup
	deadline := time.Now().Add(5 * time.Second)
	for {
		var err error
		r, err = readBrowserGroup(dir)
		if err == nil && r.PGID == cmd.Process.Pid {
			break
		}
		if gone, _ := groupGone(cmd.Process.Pid); gone {
			return func() error { return nil }, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("browser group record unavailable")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var mu sync.Mutex
	stop := make(chan struct{})
	stopped := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				mu.Lock()
				_ = snapshotGroup(dir, &r)
				mu.Unlock()
			}
		}
	}()
	return func() error {
		once.Do(func() { close(stop); <-stopped })
		mu.Lock()
		defer mu.Unlock()
		return terminateBrowserGroup(dir, r, false)
	}, nil
}

func removeBrowserGroup(dir string) error {
	err := os.Remove(filepath.Join(dir, groupRecordName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return groupError("cleanup_failed")
	}
	return nil
}
func recoverAllBrowserGroups(dir string) { _ = recoverBrowserGroups(groupCleanup{DataDir: dir}) }

func browserGroupProtocol() int { return 1 }

func browserCommandArguments(cmd *exec.Cmd) ([]string, error) {
	return processArguments(cmd.Process.Pid)
}

func recoverFailedBrowserCommand(cmd *exec.Cmd, dir string) error {
	if gone, err := groupGone(cmd.Process.Pid); err != nil {
		return groupError("cleanup_failed")
	} else if gone {
		return removeBrowserGroup(dir)
	}
	r, err := readBrowserGroup(dir)
	if err != nil || r.PGID != cmd.Process.Pid {
		return groupError("leftover_unverified")
	}
	return terminateBrowserGroup(dir, r, false)
}
