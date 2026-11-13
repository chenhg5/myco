package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
)

// An agent's face. Clients draw one from the agent's id unless people chose
// another: a drawn one with other parts ("look:..."; what the parts mean is
// up to the client, the hub only keeps the word), or a small picture.

// MaxAvatarPicture bounds a picture used as a face (clients shrink it first); its
// data: URL fits in an ordinary request body.
const MaxAvatarPicture = 192 << 10

var avatarLook = regexp.MustCompile(`^look:[a-z0-9-]{1,48}$`)

// avatarTypes are the pictures a face may be: plain raster images that every
// browser shows inline and none runs (no SVG).
var avatarTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// ParseAvatarLook checks a chosen drawn face; "" means the default one.
func ParseAvatarLook(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || avatarLook.MatchString(s) {
		return s, nil
	}
	return "", Errf(CodeInvalidArgument, "avatar must be empty, look:<parts> or a picture").
		WithSuggestion("send a PNG, JPEG, WebP or GIF picture as a data: URL, or \"\" for the default face")
}

// CheckAvatarPicture checks a picture for a face and returns its type, read
// from the bytes (what the sender claims is not trusted), and the avatar
// word that names it.
func CheckAvatarPicture(data []byte) (contentType, avatar string, err error) {
	if len(data) == 0 {
		return "", "", Errf(CodeInvalidArgument, "avatar picture is empty")
	}
	if len(data) > MaxAvatarPicture {
		return "", "", Errf(CodeInvalidArgument, "avatar picture is %d bytes, limit is %d", len(data), MaxAvatarPicture).
			WithSuggestion("use a smaller picture (a few hundred pixels square is plenty)")
	}
	ct := http.DetectContentType(data)
	if !avatarTypes[ct] {
		return "", "", Errf(CodeInvalidArgument, "avatar picture must be PNG, JPEG, WebP or GIF, not %s", ct)
	}
	sum := sha256.Sum256(data)
	return ct, "pic:" + hex.EncodeToString(sum[:8]), nil
}

// AvatarStore keeps the pictures agents use as faces, one per agent.
type AvatarStore interface {
	PutAvatar(ctx context.Context, name AgentName, contentType string, data []byte) error
	// GetAvatar returns NotFound when the agent has no picture.
	GetAvatar(ctx context.Context, name AgentName) (contentType string, data []byte, err error)
}
