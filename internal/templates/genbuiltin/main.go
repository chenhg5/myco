// Command genbuiltin writes internal/templates/builtin.zip from
// internal/templates/official (make templates runs it):
//
//	go run ./internal/templates/genbuiltin
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/myco-ai/myco/internal/templates"
)

func main() {
	dir := filepath.Join("internal", "templates")
	b, err := templates.PackBuiltin(os.DirFS(filepath.Join(dir, "official")))
	if err != nil {
		fmt.Fprintln(os.Stderr, "genbuiltin:", err)
		os.Exit(1)
	}
	out := filepath.Join(dir, "builtin.zip")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genbuiltin:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d apps, %.1f MB\n", out, len(templates.Builtin), float64(len(b))/(1<<20))
}
