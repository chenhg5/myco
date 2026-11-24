//go:build darwin || linux

package oscap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSparseDiskSizeRemainsZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sparse.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(16 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := diskOf(path, fi); got != 0 {
		t.Skipf("filesystem allocated sparse file: %d", got)
	}
	if fi.Size() != 16<<20 {
		t.Fatal("logical size changed")
	}
}
