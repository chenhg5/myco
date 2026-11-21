package domain

import (
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

// TemplateLocale is display metadata, independent of a page's user-chosen name.
// locales.json is a sidecar so hubs with a strict legacy manifest parser can
// still install and serve an app without understanding its translations.
type TemplateLocale struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

var templateLocaleTag = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$`)

func ParseTemplateLocales(raw []byte) (map[string]TemplateLocale, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 32<<10 {
		return nil, Errf(CodeInvalidArgument, "locales.json: at most 32 KiB")
	}
	var out map[string]TemplateLocale
	if json.Unmarshal(raw, &out) != nil || len(out) > 20 {
		return nil, Errf(CodeInvalidArgument, "locales.json: expected a map of at most 20 language tags")
	}
	for tag, v := range out {
		if !templateLocaleTag.MatchString(tag) {
			return nil, Errf(CodeInvalidArgument, "locales.json: invalid language tag %q", tag)
		}
		if v.Name != "" {
			if _, err := ParsePageName(v.Name); err != nil {
				return nil, Errf(CodeInvalidArgument, "locales.json: %s name must be one line, up to 80 characters", tag)
			}
		}
		if utf8.RuneCountInString(v.Description) > 300 {
			return nil, Errf(CodeInvalidArgument, "locales.json: %s description must be at most 300 characters", tag)
		}
	}
	return out, nil
}
