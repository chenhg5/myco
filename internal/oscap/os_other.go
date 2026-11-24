//go:build !darwin && !linux && !windows

package oscap

import (
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
)

// Other systems (the BSDs…): files and commands work; the rest is
// unsupported.

func isHidden(_ string, fi fs.FileInfo) bool { return strings.HasPrefix(fi.Name(), ".") }
func userDirs(home string) map[string]string {
	return map[string]string{"desktop": filepath.Join(home, "Desktop"), "downloads": filepath.Join(home, "Downloads")}
}
func fillInfo(*SystemInfo)                     {}
func disks() ([]Disk, error)                   { return nil, unsupported("system.disks") }
func isVolumeRoot(string) bool                 { return false }
func crossDevice(error) bool                   { return false }
func devOf(fs.FileInfo) (uint64, bool)         { return 0, false }
func diskOf(_ string, fi fs.FileInfo) int64    { return fi.Size() }
func inodeOf(fs.FileInfo) ([2]uint64, bool)    { return [2]uint64{}, false }
func linksOf(fs.FileInfo) uint64               { return 1 }
func shellArgv(_ *Env, script string) []string { return []string{"/bin/sh", "-c", script} }
func ownGroup(*exec.Cmd)                       {}
func hideWindow(*exec.Cmd)                     {}
func killTree(cmd *exec.Cmd) error             { return cmd.Process.Kill() }
func kill(int, bool) error                     { return unsupported("process.kill") }
func processes(context.Context, *Env) ([]Process, error) {
	return nil, unsupported("system.processes")
}
func trash(_ context.Context, _ *Env, paths []string) []error {
	errs := make([]error, len(paths))
	for i := range errs {
		errs[i] = unsupported("fs.trash")
	}
	return errs
}
func reveal(*Env, string) error                          { return unsupported("fs.reveal") }
func open(*Env, string, string) error                    { return unsupported("fs.open") }
func clipboardMissing(*Env) string                       { return "not available on this system" }
func clipboardGet(context.Context, *Env) (string, error) { return "", unsupported("clipboard") }
func clipboardSet(context.Context, *Env, string) error   { return unsupported("clipboard") }
func notifyMissing(*Env) string                          { return "not available on this system" }
func notify(context.Context, *Env, string, string, string, bool) error {
	return unsupported("notify")
}
func screenMissing(*Env) string { return "not available on this system" }
func capture(context.Context, *Env, string, int, *Region) (bool, error) {
	return false, unsupported("screen.capture")
}

func exeNames(p string) []string { return []string{p} }

func usage(context.Context, *Env) (Usage, error)    { return Usage{}, unsupported("system.usage") }
func apps(context.Context, *Env) ([]AppInfo, error) { return nil, unsupported("system.apps") }
func openSettings(*Env, string) error               { return unsupported("system.settings") }
func thumbnail(context.Context, *Env, string, int) ([]byte, error) {
	return nil, unsupported("fs.thumbnail")
}
func dialogMissing(*Env) string                              { return "not available on this system" }
func dialog(context.Context, *Env, Dialog) ([]string, error) { return nil, unsupported("fs.pick") }
