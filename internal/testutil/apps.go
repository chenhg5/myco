package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RepositoryRoot locates the host checkout independently of a test's cwd.
func RepositoryRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate host checkout")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// OfficialDir contains tiny fixed service fixtures, never real app bundles.
func OfficialDir(t testing.TB) string {
	t.Helper()
	return filepath.Join(RepositoryRoot(t), "internal", "templates", "testdata", "official")
}

// AppsRoot opts into real-Hub/native bridge tests of a separate apps checkout.
// An explicitly supplied but incomplete checkout fails instead of skipping.
func AppsRoot(t testing.TB) string {
	t.Helper()
	root := os.Getenv("APPS_TEST_ROOT")
	if root == "" {
		t.Skip("set APPS_TEST_ROOT to a built myco-ai/apps checkout for cross-repository acceptance")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"templates/node_modules/typescript/package.json", "dist/official/base/myco.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("APPS_TEST_ROOT is incomplete: %s: %v", name, err)
		}
	}
	return root
}

// AppsEnv removes ambient agent credentials; fixtures deliver their disposable
// credentials on stdin and explicitly select the real host bridge source.
func AppsEnv(t testing.TB) []string {
	t.Helper()
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "MYCO_") && !strings.HasPrefix(key, "CLAUDE") {
			env = append(env, value)
		}
	}
	return append(env, "MYCO_HOST_ROOT="+RepositoryRoot(t))
}
