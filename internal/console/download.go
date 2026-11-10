package console

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chenhg5/myco/internal/client"
)

// saveToDownloads writes a project file into the Downloads folder under a
// name not taken yet ("design (1).md"), and returns its path.
func saveToDownloads(ctx context.Context, c *client.Client, project, file string) (string, error) {
	rc, _, err := c.OpenProjectFile(ctx, project, file)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = home
	}
	name := filepath.Base(file)
	if name == "." || name == string(filepath.Separator) || strings.HasPrefix(name, ".") {
		name = "myco-file"
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; ; i++ {
		p := filepath.Join(dir, name)
		if i > 0 {
			p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(f, rc); err != nil {
			f.Close()
			os.Remove(p)
			return "", err
		}
		return p, f.Close()
	}
}

// revealFile shows a file in Finder / Explorer (best effort).
func revealFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer.exe", "/select,", path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	_ = cmd.Start()
	go func() { _ = cmd.Wait() }()
}
