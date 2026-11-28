package api

// UILanguage is a desktop/web interface preference. It never changes task contents.
type UILanguage string

func (l UILanguage) Valid() bool {
	switch l {
	case "zh", "zh-Hant", "en", "ja", "ko", "es", "pt-BR":
		return true
	}
	return false
}
