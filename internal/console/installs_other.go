//go:build !darwin

package console

// launcherKind: no launchers here yet (Windows' Start menu shortcuts need
// the running App to take a second launch's request).
var launcherKind = ""
