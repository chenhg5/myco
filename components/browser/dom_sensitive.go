package main

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

// The recorder, accessibility snapshots, extraction, input checks and pixel
// guard use the same policy. Insert only these bundled, trusted sources.
//
//go:embed dom_sensitive.json
var sensitiveConfig string

//go:embed dom_sensitive.js
var sensitiveTemplate string
var sensitiveFunction = strings.ReplaceAll(sensitiveTemplate, "__MYCO_SENSITIVE_CONFIG__", sensitiveConfig)

func withSensitivePolicy(source string) string {
	return strings.ReplaceAll(source, "__MYCO_SENSITIVE_POLICY__", sensitiveFunction)
}

var sensitivePatterns = func() struct{ Name, Autocomplete *regexp.Regexp } {
	var policy struct {
		Name         string `json:"name"`
		Autocomplete string `json:"autocomplete"`
	}
	if err := json.Unmarshal([]byte(sensitiveConfig), &policy); err != nil {
		panic("invalid sensitive field policy")
	}
	return struct{ Name, Autocomplete *regexp.Regexp }{regexp.MustCompile("(?i)" + policy.Name), regexp.MustCompile("(?i)" + policy.Autocomplete)}
}()

func sensitiveAttributes(attrs map[string]string) bool {
	_, marked := attrs["data-sensitive"]
	return marked || strings.EqualFold(attrs["type"], "password") || (!strings.EqualFold(attrs["tag"], "form") && sensitivePatterns.Name.MatchString(attrs["name"])) || sensitivePatterns.Autocomplete.MatchString(attrs["autocomplete"])
}
