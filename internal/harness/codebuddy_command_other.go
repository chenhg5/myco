//go:build !windows

package harness

func CodeBuddyCommand(binary string, args []string) (string, []string) { return binary, args }
