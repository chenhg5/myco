package oscap

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsAllocatedDiskSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "容量.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Write(make([]byte, 8193)); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if allocated := diskOf(path, fi); allocated <= 0 {
		t.Fatalf("allocated %d for logical %d", allocated, fi.Size())
	}
	var returned uint32
	if err = windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &returned, nil); err != nil {
		t.Skipf("filesystem does not support sparse files: %v", err)
	}
	// A logical length beyond 32 bits must not become physical storage.
	if err = f.Truncate(5 << 30); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Lstat(path)
	if allocated := diskOf(path, fi); allocated <= 0 || allocated >= 5<<30 {
		t.Fatalf("sparse allocation %d, logical %d", allocated, fi.Size())
	}
	if err = f.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(5 << 30); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Lstat(path)
	if allocated := diskOf(path, fi); allocated != 0 {
		t.Fatalf("empty sparse allocation = %d, want 0", allocated)
	}
}
