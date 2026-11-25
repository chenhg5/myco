package main

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// Native tests execute the same private launch/recovery entry points as the
// installed component rather than substituting a different process protocol.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--test-component" {
		if run(context.Background(), os.Stdin, os.Stdout) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if handled, err := browserInternalMode(os.Args, os.Stdin); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "private helper failed:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if groupTestMode() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
