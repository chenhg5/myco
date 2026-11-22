package remotedesktop

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, text := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = w.Write([]byte(text))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestComponentArchiveIsConfinedAndComplete(t *testing.T) {
	for _, files := range []map[string]string{{"../escaped": "bad"}, {"x\\escaped": "bad"}, {program("myco-remote"): "only engine"}, {program("myco-remote"): "engine", program("ffmpeg"): "encoder", "run.sh": "unexpected"}} {
		if e := extractComponent(archive(t, files), t.TempDir()); e == nil {
			t.Fatal("unsafe archive accepted", files)
		}
	}
	dir := t.TempDir()
	if e := extractComponent(archive(t, map[string]string{program("myco-remote"): "engine", program("ffmpeg"): "encoder", "LICENSE-FFmpeg.txt": "LGPL"}), dir); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dir, program("myco-remote"))); e != nil {
		t.Fatal(e)
	}
}
func TestComponentIsAbsentByDefault(t *testing.T) {
	c := &Component{Home: t.TempDir()}
	s := c.Status()
	if s.Installed || s.Running || s.PairingCode != "" {
		t.Fatal(s)
	}
	c.Stop()
	if e := c.Remove(); e != nil {
		t.Fatal(e)
	}
}

func TestComponentRejectsInsecureAddress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	c := &Component{Home: t.TempDir()}
	if e := os.MkdirAll(c.dir(), 0700); e != nil {
		t.Fatal(e)
	}
	script := "#!/bin/sh\nread config\nprintf '%s\\n' '{\"address\":\"example.org:1234\",\"protocol\":1}'\ncat >/dev/null\n"
	if e := os.WriteFile(filepath.Join(c.dir(), program("myco-remote")), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(c.dir(), program("ffmpeg")), []byte("fixture"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Start(context.Background(), false, nil); e == nil {
		t.Fatal("non-loopback component accepted")
	}
	if c.Status().Running {
		t.Fatal("failed process was kept")
	}
}

func TestComponentNeverUsesWorkingDirectoryAsHome(t *testing.T) {
	c := &Component{}
	if c.Status().Supported || c.Status().Installed {
		t.Fatal(c.Status())
	}
	if c.Remove() == nil {
		t.Fatal("removed from an implicit relative directory")
	}
	if _, e := c.Start(context.Background(), false, nil); e == nil {
		t.Fatal("started without an absolute home")
	}
}

func installComponentFixture(t *testing.T, c *Component, version, script string) string {
	t.Helper()
	dir := filepath.Join(c.Home, ".myco", "components", "remote", version)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{program("myco-remote"): script, program("ffmpeg"): "fixture"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestComponentReusesInstalledCompatibleRelease(t *testing.T) {
	c := &Component{Home: t.TempDir()}
	old := installComponentFixture(t, c, "0.1.0", "old engine")
	if got := c.Status(); !got.Installed || got.Version != "0.1.0" || !got.UpdateAvailable {
		t.Fatal(got)
	}
	// An incomplete current install must not conceal a working old one.
	if err := os.MkdirAll(filepath.Join(c.dir(), program("ffmpeg")), 0700); err != nil {
		t.Fatal(err)
	}
	if got := c.Status(); !got.Installed || got.Version != "0.1.0" {
		t.Fatal(got)
	}
	if err := os.RemoveAll(c.dir()); err != nil {
		t.Fatal(err)
	}
	installComponentFixture(t, c, ComponentVersion, "new engine")
	if got := c.Status(); !got.Installed || got.Version != ComponentVersion || got.UpdateAvailable {
		t.Fatal(got)
	}
	unknown := installComponentFixture(t, c, "99.0.0", "unknown engine")
	if err := c.Remove(); err != nil {
		t.Fatal(err)
	}
	if got := c.Status(); got.Installed {
		t.Fatal(got)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old release remains: %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown release removed: %v", err)
	}
}

func TestComponentStartsCompatibleReleaseWithoutDownloading(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	c := &Component{Home: t.TempDir()}
	installComponentFixture(t, c, "0.1.0", `#!/bin/sh
read config
printf '%s\n' '{"address":"127.0.0.1:12345","protocol":1}'
cat >/dev/null
`)
	defer c.Stop()
	got, err := c.Start(context.Background(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Running || got.Version != "0.1.0" {
		t.Fatal(got)
	}
}
