//go:build !windows

package console

func bundledAgentLogin(path string) string { return shellQuote(path) }
