package quota

import (
	"encoding/json"
	"os"
)

// readJSONC accepts OpenCode's JSON-with-comments format without altering
// quoted URLs, key strings, or escapes. Errors never include configuration.
func readJSONC(path string, v any) bool {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 4<<20 {
		return false
	}
	out := make([]byte, 0, len(b))
	quoted, escape := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if quoted {
			out = append(out, c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(b) {
			if b[i+1] == '/' {
				for i < len(b) && b[i] != '\n' {
					i++
				}
				out = append(out, '\n')
				continue
			}
			if b[i+1] == '*' {
				i += 2
				for i+1 < len(b) && (b[i] != '*' || b[i+1] != '/') {
					i++
				}
				i++
				out = append(out, ' ')
				continue
			}
		}
		out = append(out, c)
	}
	b = out
	out = make([]byte, 0, len(b))
	quoted, escape = false, false
	for i, c := range b {
		if quoted {
			out = append(out, c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
		}
		if c == ',' {
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\n' || b[j] == '\r' || b[j] == '\t') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return json.Unmarshal(out, v) == nil
}
