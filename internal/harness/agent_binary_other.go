//go:build !darwin && !windows && !linux

package harness

import "os"

func WorkBuddyBinaries(home string) []string { return nil }
func bundledExecutable(st os.FileInfo) bool  { return st.Mode()&0o111 != 0 }
