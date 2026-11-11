package harness

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// forkPrefix marks a node's session that is a copy still to be made: the
// node joined at once, and its first run copies the source session (Fork)
// instead of a separate turn spent only on making the copy.
const forkPrefix = "fork:"

// ForkMarker is the session id of a node that copies src on its first run.
// Each marker is unique (two copies of one session are two nodes).
func ForkMarker(src string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return forkPrefix + src + ":" + hex.EncodeToString(b)
}

// ForkSource is the session a marker copies, and whether session is one.
func ForkSource(session string) (string, bool) {
	rest, ok := strings.CutPrefix(session, forkPrefix)
	if !ok {
		return "", false
	}
	i := strings.LastIndex(rest, ":")
	if i <= 0 {
		return "", false
	}
	return rest[:i], true
}
