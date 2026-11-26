package console

import "strings"

func bundledAgentLogin(path string) string { return "& '" + strings.ReplaceAll(path, "'", "''") + "'" }
