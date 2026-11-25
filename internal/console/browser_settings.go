package console

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"

	"github.com/myco-ai/myco/internal/api"
)

type browserConfigure struct {
	ID       string                           `json:"id"`
	Settings *api.BrowserProfileSettingsInput `json:"settings"`
}

// Bind the one-use host confirmation to the entire typed input, including
// write-only credentials. This digest is private in-memory nonce state only.
func browserConfigureBound(raw json.RawMessage) (browserConfigure, string, error) {
	var in browserConfigure
	if err := browserDecode(raw, &in); err != nil {
		return in, "", err
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`).MatchString(in.ID) || in.Settings == nil {
		return in, "", browserFailure("invalid_argument", 400)
	}
	canonical, err := json.Marshal(in)
	if err != nil {
		return in, "", browserFailure("invalid_argument", 400)
	}
	sum := sha256.Sum256(canonical)
	return in, in.ID + "\x00" + hex.EncodeToString(sum[:]), nil
}
