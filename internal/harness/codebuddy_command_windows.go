package harness

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// CodeBuddyCommand avoids cmd.exe interpolation of prompts/options. WorkBuddy
// ships a Node script next to its .cmd launcher; npm puts it under node_modules.
// Run that exact script through Node, retaining ordinary argv boundaries.
func CodeBuddyCommand(binary string, args []string) (string, []string) {
	if !strings.EqualFold(filepath.Base(binary), "codebuddy.cmd") {
		return binary, args
	}
	dir := filepath.Dir(binary)
	for _, script := range []string{filepath.Join(dir, "codebuddy"), filepath.Join(dir, "node_modules", "@tencent-ai", "codebuddy-code", "bin", "codebuddy")} {
		f, err := os.Open(script)
		if err != nil {
			continue
		}
		var head [96]byte
		n, _ := f.Read(head[:])
		f.Close()
		if !bytes.HasPrefix(head[:n], []byte("#!/usr/bin/env node")) && !bytes.HasPrefix(head[:n], []byte("#!/usr/bin/node")) {
			continue
		}
		node := filepath.Join(dir, "node.exe")
		if st, err := os.Stat(node); err != nil || st.IsDir() {
			node = "node"
		}
		return node, append([]string{script}, args...)
	}
	return binary, args
}
