package dbstudio

import (
	"strings"
	"unicode"
)

// singleStatement understands quoted identifiers, comments, PostgreSQL dollar
// strings and E-strings. Driver transactions still enforce read-only execution.
// Executable MySQL comments are refused rather than interpreted as whitespace.
func singleStatement(input, engine string) (string, error) {
	s := strings.TrimSpace(strings.TrimPrefix(input, "\ufeff"))
	if len(s) == 0 || len(s) > 1<<20 || strings.ContainsRune(s, 0) {
		return "", failure("invalid_argument", "enter one statement of at most 1 MiB")
	}
	first := ""
	ended := false
	for i := 0; i < len(s); {
		if unicode.IsSpace(rune(s[i])) {
			i++
			continue
		}
		if i+1 < len(s) && s[i:i+2] == "--" {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if engine == "mysql" && s[i] == '#' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(s) && s[i:i+2] == "/*" {
			if i+2 < len(s) && (s[i+2] == '!' || s[i+2] == '+') {
				return "", failure("invalid_argument", "executable SQL comments are not supported")
			}
			i += 2
			depth := 1
			for i < len(s) && depth > 0 {
				if i+1 < len(s) && s[i:i+2] == "/*" {
					depth++
					i += 2
				} else if i+1 < len(s) && s[i:i+2] == "*/" {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return "", failure("invalid_argument", "unclosed SQL comment")
			}
			continue
		}
		if ended {
			return "", failure("invalid_argument", "run one SQL statement at a time")
		}
		if s[i] == ';' {
			ended = true
			i++
			continue
		}
		if s[i] == '\'' || s[i] == '"' || s[i] == '`' || (engine == "sqlite" && s[i] == '[') {
			q := s[i]
			close := q
			if q == '[' {
				close = ']'
			}
			escape := engine == "mysql" || (engine == "postgres" && q == '\'' && i > 0 && (s[i-1] == 'e' || s[i-1] == 'E') && (i == 1 || !isWord(s[i-2])))
			i++
			closed := false
			for i < len(s) {
				if escape && s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == close {
					if i+1 < len(s) && s[i+1] == close {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", failure("invalid_argument", "unclosed SQL string or identifier")
			}
			continue
		}
		if engine == "postgres" && s[i] == '$' {
			j := i + 1
			for j < len(s) && isWord(s[j]) && s[j] != '$' {
				j++
			}
			if j < len(s) && s[j] == '$' {
				tag := s[i : j+1]
				end := strings.Index(s[j+1:], tag)
				if end < 0 {
					return "", failure("invalid_argument", "unclosed dollar-quoted string")
				}
				i = j + 1 + end + len(tag)
				continue
			}
		}
		if isWord(s[i]) {
			j := i + 1
			for j < len(s) && isWord(s[j]) {
				j++
			}
			if first == "" {
				first = strings.ToUpper(s[i:j])
			}
			i = j
			continue
		}
		i++
	}
	if first == "" {
		return "", failure("invalid_argument", "enter a SQL statement")
	}
	return first, nil
}
func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
func queryStatement(sql, engine string) error {
	word, err := singleStatement(sql, engine)
	if err != nil {
		return err
	}
	switch word {
	case "SELECT", "WITH", "EXPLAIN", "SHOW", "DESCRIBE", "DESC":
		return nil
	}
	return failure("permission_denied", "use Query for read-only SQL; choose Execute on a writable connection for changes")
}
