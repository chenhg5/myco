//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func groupTestMode() bool {
	if len(os.Args) == 3 && os.Args[1] == "--test-inherited-lease" {
		lease := os.NewFile(3, "profile-lease")
		if lease == nil {
			os.Exit(2)
		}
		info, err := lease.Stat()
		if err != nil || !info.Mode().IsRegular() {
			os.Exit(3)
		}
		if os.WriteFile(os.Args[2], []byte("ready"), 0600) != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if len(os.Args) < 3 || os.Args[1] != "--group-test" {
		return false
	}
	signal.Ignore(syscall.SIGTERM)
	if os.Args[2] == "child" {
		for {
			time.Sleep(time.Second)
		}
	}
	cmd := exec.Command(os.Args[0], "--group-test", "child")
	if err := cmd.Start(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[2], []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		os.Exit(2)
	}
	for {
		time.Sleep(time.Second)
	}
}
func groupFixture(t *testing.T, profileArgument ...bool) (string, *exec.Cmd, browserGroup, chan struct{}) {
	t.Helper()
	if err := adoptBrowserChildren(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "profiles", "p_fixture")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, canonicalErr := filepath.EvalSymlinks(dir)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	dir = canonical
	marker := filepath.Join(dir, "child.pid")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--group-test", marker}
	if len(profileArgument) > 0 && profileArgument[0] {
		args = append(args, "--user-data-dir="+dir)
	}
	cmd := exec.Command(self, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	owner, err := ownProcess()
	if err != nil {
		t.Fatal(err)
	}
	members, err := groupProcesses(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	r := browserGroup{Version: 1, Profile: filepath.Base(dir), Nonce: "test-fixture-generation", PGID: cmd.Process.Pid, Owner: owner, Members: members}
	if err = saveBrowserGroup(dir, r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-r.PGID, syscall.SIGKILL)
		for i := 0; i < 100; i++ {
			for _, p := range r.Members {
				if p.PID != r.PGID {
					reapBrowserChild(p.PID)
				}
			}
			if gone, _ := groupGone(r.PGID); gone {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return dir, cmd, r, done
}
func TestBrowserGroupLeaseWaitsForAllDescendants(t *testing.T) {
	dir, cmd, r, done := groupFixture(t)
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := guardBrowser(cmd, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &engine{sessions: map[string]*session{}}
	s := &session{e: e, id: "p_fixture", ctx: ctx, cancel: cancel, cmd: cmd, done: done, lock: lease, guard: cleanup}
	e.sessions[s.id] = s
	finished := make(chan error, 1)
	go func() { finished <- s.close() }()
	// Both fixture processes ignore TERM. The lease must survive that grace period.
	time.Sleep(150 * time.Millisecond)
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e == nil {
		extra.Close()
		t.Fatal("lease released while descendants were alive")
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("group shutdown did not complete")
	}
	if gone, e := groupGone(r.PGID); e != nil || !gone {
		t.Fatalf("group remains: gone=%v err=%v", gone, e)
	}
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e != nil {
		t.Fatal("lease not released", e)
	} else {
		extra.Close()
	}
	if _, e := os.Stat(filepath.Join(dir, groupRecordName)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("group record not removed", e)
	}
}
func TestBrowserGroupRecoveryRejectsReusedIdentity(t *testing.T) {
	dir, _, r, _ := groupFixture(t)
	for i := range r.Members {
		r.Members[i].Start = "different-kernel-generation"
	}
	if err := saveBrowserGroup(dir, r); err != nil {
		t.Fatal(err)
	}
	err := recoverProfileProcesses(dir, nil)
	var locked *profileProcessError
	if !errors.As(err, &locked) || locked.Reason != "leftover_unverified" {
		t.Fatalf("unexpected recovery: %v", err)
	}
	if gone, _ := groupGone(r.PGID); gone {
		t.Fatal("unverified group was killed")
	}
}
func TestBrowserGroupRecoveryEmptyRecord(t *testing.T) {
	dir, cmd, r, done := groupFixture(t)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	<-done
	for i := 0; i < 100; i++ {
		for _, p := range r.Members {
			reapBrowserChild(p.PID)
		}
		if gone, _ := groupGone(r.PGID); gone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := recoverProfileProcesses(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, groupRecordName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("empty record retained", err)
	}
}
func TestBrowserPrivateLaunchRejectsOtherExecutable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "p_fixture")
	os.MkdirAll(dir, 0700)
	raw, _ := json.Marshal(browserLaunch{Binary: "/not/an/installed/browser", ProfileDir: dir, Args: []string{"--user-data-dir=" + dir}})
	cmd := exec.Command(os.Args[0], "--launch-browser")
	cmd.Stdin = bytes.NewReader(raw)
	if err := cmd.Run(); err == nil {
		t.Fatal("arbitrary executable accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, groupRecordName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected launch wrote record")
	}
}

func TestBrowserGroupRecoveryFallsBackToExactProfileArgument(t *testing.T) {
	dir, _, r, _ := groupFixture(t, true)
	for i := range r.Members {
		r.Members[i].Start = "old-member-snapshot"
	}
	if err := saveBrowserGroup(dir, r); err != nil {
		t.Fatal(err)
	}
	if err := recoverProfileProcesses(dir, nil); err != nil {
		t.Fatal(err)
	}
	if gone, err := groupGone(r.PGID); err != nil || !gone {
		t.Fatalf("fallback failed: %v %v", gone, err)
	}
}

func TestBrowserGroupRecoverySkipsAnotherLiveLease(t *testing.T) {
	dir, _, r, _ := groupFixture(t)
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = recoverBrowserGroups(groupCleanup{DataDir: filepath.Dir(filepath.Dir(dir))}); err != nil {
		t.Fatal(err)
	}
	if gone, _ := groupGone(r.PGID); gone {
		t.Fatal("another active profile was killed")
	}
}
func TestBrowserGroupProfileArgumentIsNotAPrefixMatch(t *testing.T) {
	dir, _, r, _ := groupFixture(t, true)
	all, err := groupProcesses(r.PGID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.Members {
		r.Members[i].Start = "old-snapshot"
	}
	if verifiedGroup(dir+"-another", r, all) {
		t.Fatal("accepted a similar profile path")
	}
}

func TestBrowserLaunchShimInheritsProfileLease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "p_fixture")
	os.MkdirAll(dir, 0700)
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := newBrowserCommand("unused-by-fixture", nil, dir, lease)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "shim-ready")
	cmd.Args = []string{cmd.Path, "--test-inherited-lease", marker}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shim did not inherit lease")
		}
		time.Sleep(10 * time.Millisecond)
	}
	lease.Close()
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e == nil {
		extra.Close()
		t.Fatal("parent exit would release an in-flight shim lease")
	}
	_ = cmd.Process.Kill()
	<-done
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e != nil {
		t.Fatal("shim exit did not release lease", e)
	} else {
		extra.Close()
	}
}

func TestBrowserRecoveryWaitsForDeadOwnersLaunchLease(t *testing.T) {
	dir, _, r, _ := groupFixture(t)
	r.Owner = groupProcess{PID: 999999999, Start: "exited-fixture-owner"}
	if err := saveBrowserGroup(dir, r); err != nil {
		t.Fatal(err)
	}
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(100 * time.Millisecond); lease.Close() }()
	if err = recoverBrowserGroups(groupCleanup{DataDir: filepath.Dir(filepath.Dir(dir)), OwnerPID: r.Owner.PID}); err != nil {
		t.Fatal(err)
	}
	if gone, _ := groupGone(r.PGID); !gone {
		t.Fatal("App missed a just-orphaned launcher")
	}
}
func TestBrowserFailedCloseRetainsLeaseAndCanRetry(t *testing.T) {
	dir, cmd, r, done := groupFixture(t)
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &engine{sessions: map[string]*session{}}
	retry := false
	s := &session{e: e, id: "p_fixture", ctx: ctx, cancel: cancel, cmd: cmd, done: done, lock: lease, guard: func() error {
		if !retry {
			return groupError("cleanup_pending")
		}
		_ = syscall.Kill(-r.PGID, syscall.SIGKILL)
		return terminateBrowserGroup(dir, r, false)
	}}
	e.sessions[s.id] = s
	if err = s.close(); err == nil {
		t.Fatal("failed cleanup reported success")
	}
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e == nil {
		extra.Close()
		t.Fatal("failed cleanup released lease")
	}
	retry = true
	if err = s.close(); err != nil {
		t.Fatalf("cleanup cannot retry: %v; cause=%v", err, errors.Unwrap(err))
	}
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e != nil {
		t.Fatal("successful retry retained lease", e)
	} else {
		extra.Close()
	}
}

func TestBrowserRecoveryDoesNotKillAnotherLiveOwner(t *testing.T) {
	dir, _, record, _ := groupFixture(t)
	_, _, other, _ := groupFixture(t)
	record.Owner = other.Members[0]
	if err := saveBrowserGroup(dir, record); err != nil {
		t.Fatal(err)
	}
	if err := recoverProfileProcesses(dir, nil); err == nil {
		t.Fatal("recovered another live owner's browser")
	}
	if gone, _ := groupGone(record.PGID); gone {
		t.Fatal("another live owner's browser was killed")
	}
}

func TestBrowserCloseDuringGuardInstallation(t *testing.T) {
	dir, cmd, record, done := groupFixture(t)
	lease, err := lockProfile(filepath.Join(dir, ".myco-lease"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &engine{dir: filepath.Dir(filepath.Dir(dir)), sessions: map[string]*session{}}
	// The process is started and recorded, but its normal guard is not yet set.
	s := &session{e: e, id: "p_fixture", ctx: ctx, cancel: cancel, cmd: cmd, done: done, lock: lease}
	e.sessions[s.id] = s
	finished := make(chan error, 1)
	go func() { finished <- s.close() }()
	time.Sleep(150 * time.Millisecond)
	if extra, e := lockProfile(filepath.Join(dir, ".myco-lease")); e == nil {
		extra.Close()
		t.Fatal("startup cancellation released a live group's lease")
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("startup group cleanup did not finish")
	}
	if gone, _ := groupGone(record.PGID); !gone {
		t.Fatal("startup cancellation left descendants")
	}
}

func TestBrowserRecordRejectsUnsafeGroupNumbers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "p_fixture")
	os.MkdirAll(dir, 0700)
	maxPID := int64(1<<31 - 1)
	for _, value := range []int64{-1, 0, 1, maxPID + 1, 1 << 62} {
		r := browserGroup{Version: 1, Nonce: "invalid-group-fixture", PGID: int(value), Members: []groupProcess{{PID: 2, PGID: int(value), Start: "fixture"}}}
		if err := saveBrowserGroup(dir, r); err != nil {
			t.Fatal(err)
		}
		if _, err := readBrowserGroup(dir); err == nil {
			t.Fatalf("unsafe group number accepted: %d", value)
		}
	}
}

func TestCopiedProfileRecordCannotKillSourceBrowser(t *testing.T) {
	dir, _, record, _ := groupFixture(t)
	copyDir := filepath.Join(filepath.Dir(dir), "p_another")
	if err := os.MkdirAll(copyDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := saveBrowserGroup(copyDir, record); err != nil {
		t.Fatal(err)
	}
	if err := recoverProfileProcesses(copyDir, nil); err == nil {
		t.Fatal("copied record authorized another profile's group")
	}
	if gone, _ := groupGone(record.PGID); gone {
		t.Fatal("source profile browser was killed")
	}
}

// Chrome is asked to quit before its helpers: a SIGTERM to the whole group
// kills helpers under a browser still running, which crashes it ("Google
// Chrome quit unexpectedly" on macOS). Here the leader ends its helper
// itself; the helper notes a SIGTERM that reaches it directly.
func TestBrowserGroupStopsLeaderFirst(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "p_leader")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	dir, _ = filepath.EvalSymlinks(dir)
	// Every process starts before the group is recorded and lives until
	// signalled (nothing short-lived: other tests make this process a
	// subreaper, and an unrecorded orphan would never be reaped).
	script := `M="$1"
sh -c 's=; trap "echo term > \"$0/helper-term\"; kill \$s; exit 0" TERM; sleep 1000 & s=$!; echo ready > "$0/helper.ready"; wait $s' "$M" &
c=$!
sleep 1000 &
s=$!
trap 'kill -KILL $c $s; exit 0' TERM
while [ ! -f "$M/helper.ready" ]; do :; done
echo $c > "$M/helper.pid"
wait $s`
	cmd := exec.Command("/bin/sh", "-c", script, "leader", dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	for i := 0; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, "helper.pid")); err == nil {
			break
		}
		if i > 500 {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	owner, err := ownProcess()
	if err != nil {
		t.Fatal(err)
	}
	members, err := groupProcesses(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	r := browserGroup{Version: 1, Profile: filepath.Base(dir), Nonce: "test-fixture-generation", PGID: cmd.Process.Pid, Owner: owner, Members: members}
	if err := saveBrowserGroup(dir, r); err != nil {
		t.Fatal(err)
	}
	if err := terminateBrowserGroup(dir, r, false); err != nil {
		t.Fatal(err)
	}
	<-done
	if _, err := os.Stat(filepath.Join(dir, "helper-term")); err == nil {
		t.Fatal("the helper was sent SIGTERM directly, not ended by its browser")
	}
}
