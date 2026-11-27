package console

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func preferenceCall(t *testing.T, b *Backend, method, path string, body any, status int) api.LocalPreferences {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	b.Handler().ServeHTTP(r, httptest.NewRequest(method, path, bytes.NewReader(raw)))
	if r.Code != status {
		t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body)
	}
	var out api.LocalPreferences
	if status == 200 && !strings.Contains(path, "/images") {
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}
func preferenceValue(t *testing.T, p api.LocalPreferences, key, want string) {
	t.Helper()
	if p.Values[key] == nil || *p.Values[key] != want {
		t.Fatalf("%s = %v; want %q", key, p.Values[key], want)
	}
}
func TestLocalPreferencesSurviveRestartAndImportOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	b := &Backend{Store: NewStore(path)}
	preferenceCall(t, b, "GET", "/app/preferences", nil, 200)
	preferenceCall(t, b, "POST", "/app/preferences/import", map[string]any{"values": map[string]any{"myco-theme": "dark", "myco-usage-days": "30"}}, 200)
	preferenceCall(t, b, "PATCH", "/app/preferences", map[string]any{"values": map[string]any{"myco-theme": "light", "myco-usage-days": nil}}, 200)
	// A restarted App / another WebView origin reads the same profile. Its stale
	// browser migration cannot replace either a saved value or a deletion.
	b = &Backend{Store: NewStore(path)}
	out := preferenceCall(t, b, "POST", "/app/preferences/import", map[string]any{"values": map[string]any{"myco-theme": "dark", "myco-usage-days": "90", "myco-lang": "en"}}, 200)
	preferenceValue(t, out, "myco-theme", "light")
	preferenceValue(t, out, "myco-lang", "en")
	if v, ok := out.Values["myco-usage-days"]; !ok || v != nil {
		t.Fatal("missing deletion tombstone")
	}
	other := &Backend{Store: NewStore(filepath.Join(filepath.Dir(path), "other.json"))}
	if out := preferenceCall(t, other, "GET", "/app/preferences", nil, 200); len(out.Values) != 0 {
		t.Fatal("preferences crossed profiles")
	}
	for _, key := range []string{"provider-token", "myco-auth", "myco-app-hubs", "myco-app-sidebar"} {
		preferenceCall(t, b, "PATCH", "/app/preferences", map[string]any{"values": map[string]string{key: "do not copy"}}, 400)
	}
	st, err := os.Stat(filepath.Join(b.preferenceDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
		t.Fatalf("private file mode: %v", st.Mode())
	}
}
func TestLocalWorkspacePicturesAreFilesAndEditsMerge(t *testing.T) {
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json"))}
	// A tiny neutral PNG fixture, not a user's picture.
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	pic := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	out := preferenceCall(t, b, "POST", "/app/preferences/import", map[string]any{"values": map[string]string{"myco-app-hub-looks": fmt.Sprintf(`{"demo":{"pic":%q}}`, pic)}}, 200)
	var looks map[string]struct {
		Pic string `json:"pic"`
	}
	if err := json.Unmarshal([]byte(*out.Values["myco-app-hub-looks"]), &looks); err != nil {
		t.Fatal(err)
	}
	url := looks["demo"].Pic
	if !strings.HasPrefix(url, preferenceImageURL) {
		t.Fatal("not a local file reference")
	}
	r := httptest.NewRecorder()
	b.Handler().ServeHTTP(r, httptest.NewRequest("GET", url, nil))
	if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), png) || r.Header().Get("Content-Type") != "image/png" {
		t.Fatal("picture not served")
	}
	for _, id := range []string{"first", "second"} {
		preferenceCall(t, b, "PATCH", "/app/preferences/hubs/"+id, map[string]any{"name": id, "look": map[string]any{"color": 2, "pic": url}}, 200)
	}
	out = preferenceCall(t, &Backend{Store: NewStore(b.Store.path)}, "GET", "/app/preferences", nil, 200)
	var names map[string]string
	if err := json.Unmarshal([]byte(*out.Values["myco-app-hub-names"]), &names); err != nil {
		t.Fatal(err)
	}
	if names["first"] != "first" || names["second"] != "second" {
		t.Fatal("editing another workspace overwrote its alias")
	}
	raw, err := os.ReadFile(filepath.Join(b.preferenceDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("data:image")) || bytes.Contains(raw, []byte("iVBOR")) {
		t.Fatal("picture bytes in metadata")
	}
	before := string(raw)
	preferenceCall(t, b, "PATCH", "/app/preferences/hubs/first", map[string]any{"name": "lost", "look": map[string]string{"pic": "https://example.com/private.png"}}, 400)
	after, _ := os.ReadFile(filepath.Join(b.preferenceDir(), "preferences.json"))
	if string(after) != before {
		t.Fatal("invalid picture damaged previous settings")
	}
}
func TestLocalPreferencesPreserveUnreadableAndFailedSaves(t *testing.T) {
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "profile.json"))}
	preferenceCall(t, b, "PATCH", "/app/preferences", map[string]any{"values": map[string]string{"myco-theme": "dark"}}, 200)
	path := filepath.Join(b.preferenceDir(), "preferences.json")
	// A competing App process holds the profile lock: retry, never overwrite.
	unlock, err := lockPageOrigins(filepath.Join(b.preferenceDir(), "preferences.lock"))
	if err != nil {
		t.Fatal(err)
	}
	preferenceCall(t, b, "PATCH", "/app/preferences", map[string]any{"values": map[string]string{"myco-theme": "light"}}, 503)
	unlock()
	preferenceValue(t, preferenceCall(t, b, "GET", "/app/preferences", nil, 200), "myco-theme", "dark")
	if err := os.WriteFile(path, []byte("corrupt preserved data"), 0600); err != nil {
		t.Fatal(err)
	}
	preferenceCall(t, b, "GET", "/app/preferences", nil, 500)
	preferenceCall(t, b, "POST", "/app/preferences/import", map[string]any{"values": map[string]string{"myco-theme": "light"}}, 500)
	raw, _ := os.ReadFile(path)
	if string(raw) != "corrupt preserved data" {
		t.Fatal("corrupt file replaced with defaults")
	}
	// Picture file I/O failure is reported, while valid metadata remains intact.
	if err := os.WriteFile(path, []byte(`{"version":1,"values":{"myco-theme":"dark"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.preferenceDir(), "images"), []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	pic := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n"))
	preferenceCall(t, b, "PATCH", "/app/preferences/hubs/demo", map[string]any{"name": "not committed", "look": map[string]string{"pic": pic}}, 500)
	preferenceValue(t, preferenceCall(t, b, "GET", "/app/preferences", nil, 200), "myco-theme", "dark")
	// A path occupied by a directory cannot be atomically replaced by a file.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	preferenceCall(t, b, http.MethodPatch, "/app/preferences", map[string]any{"values": map[string]string{"myco-theme": "light"}}, 500)
}
