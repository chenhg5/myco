//go:build !windows && !darwin

package sysproxy

// readSystem: Linux desktops keep proxies in the environment, which Go
// reads by itself.
func readSystem() Setting { return Setting{} }
