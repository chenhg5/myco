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
