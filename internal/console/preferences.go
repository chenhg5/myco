package console

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/myco-ai/myco/internal/api"
)

const preferencesLimit = 24 << 20
const preferenceImageLimit = 8 << 20

// An explicit list: never import credentials, provider settings, arbitrary
// application storage, or the workspace's cached conversations into this file.
var preferenceKeys = map[string]bool{
	"myco-app-hub-looks": true, "myco-app-hub-names": true,
	"myco-theme": true, "myco-lang": true, "myco-text-size": true, "myco-ui-style": true,
	"myco-nav-hidden": true, "myco-nav-chat-sort": true, "myco-conv-pins": true,
	"myco-focus-bg": true, "myco-chat-bg": true, "myco-simple-mode": true,
	"myco-island": true, "myco-runs-reports-only": true, "myco-translate-auto": true,
	"myco-usage-days": true, "myco-drafts": true, "myco-queued": true,
	"myco-onboard": true, "myco-checklist-off": true, "myco-intro-seen": true,
	"myco-onboard-did": true, "myco-starters-off": true, "myco-plan-bar-closed": true, "myco-page-recovery-completed": true,
}
var preferencePrefixes = []string{"myco-usage-project:", "myco-nav-pinned:", "myco-nav-order\x00", "myco-hidden-", "myco-sent\x00"}

func allowedPreference(key string) bool {
	if len(key) > 1024 {
		return false
	}
	if preferenceKeys[key] {
		return true
	}
	for _, p := range preferencePrefixes {
		if strings.HasPrefix(key, p) && len(key) > len(p) {
			return true
		}
	}
	return false
}

func (b *Backend) preferenceDir() string {
	if b.Store == nil || b.Store.path == "" {
		return ""
	}
	// The Store path identifies a local App profile; separate stores in one
	// directory must not share their preferences or private pictures.
	base := strings.TrimSuffix(filepath.Base(b.Store.path), filepath.Ext(b.Store.path))
	return filepath.Join(filepath.Dir(b.Store.path), base+"-preferences")
}
func (b *Backend) readPreferences() (api.LocalPreferences, error) {
	out := api.LocalPreferences{Version: 1, Values: map[string]*string{}}
	dir := b.preferenceDir()
	if dir == "" {
		return out, errors.New("local app profile is not configured")
	}
	f, err := os.Open(filepath.Join(dir, "preferences.json"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, preferencesLimit+1))
	if err != nil {
		return out, err
	}
	if len(data) > preferencesLimit {
		return out, errors.New("preferences file is too large")
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return out, errors.New("preferences file is invalid; the original has been preserved")
	}
	if out.Version != 1 {
		return out, errors.New("preferences file version is not supported")
	}
	if out.Values == nil {
		out.Values = map[string]*string{}
	}
	return out, nil
}
func preferenceAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func preferenceFailure(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": "local_preferences", "message": message, "retryable": status >= 500})
}

func (b *Backend) preferenceRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /app/preferences", func(w http.ResponseWriter, r *http.Request) {
		b.preferenceMu.Lock()
		defer b.preferenceMu.Unlock()
		out, err := b.readPreferences()
		if err != nil {
			preferenceFailure(w, 500, "Could not read local preferences. No saved data was changed.")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, out)
	})
	patch := func(importOnly bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var in api.LocalPreferencesPatch
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, preferencesLimit)).Decode(&in); err != nil {
				preferenceFailure(w, 400, "Invalid local preferences request.")
				return
			}
			if len(in.Values) > 10000 {
				preferenceFailure(w, 400, "Too many preference keys.")
				return
			}
			for key := range in.Values {
				if !allowedPreference(key) {
					preferenceFailure(w, 400, "This preference key is not supported.")
					return
				}
			}
			b.updatePreferences(w, func(out *api.LocalPreferences) error {
				for key, value := range in.Values {
					if _, exists := out.Values[key]; importOnly && exists {
						continue
					}
					if value != nil {
						normal, err := b.normalizePreference(key, *value)
						if err != nil {
							return errors.New("invalid preference or picture")
						}
						value = &normal
					}
					// Null tombstones stop a stale window resurrecting deleted choices.
					out.Values[key] = value
				}
				return nil
			})
		}
	}
	m.HandleFunc("PATCH /app/preferences", patch(false))
	m.HandleFunc("POST /app/preferences/import", patch(true))
	m.HandleFunc("PATCH /app/preferences/hubs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in api.LocalWorkspaceAppearance
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, preferencesLimit)).Decode(&in); err != nil || len(in.Name) > 1024 || len(r.PathValue("id")) > 1024 {
			preferenceFailure(w, 400, "Invalid workspace appearance.")
			return
		}
		b.updatePreferences(w, func(out *api.LocalPreferences) error {
			names := map[string]string{}
			looks := map[string]json.RawMessage{}
			if raw := out.Values["myco-app-hub-names"]; raw != nil {
				if err := json.Unmarshal([]byte(*raw), &names); err != nil {
					return err
				}
			}
			if raw := out.Values["myco-app-hub-looks"]; raw != nil {
				if err := json.Unmarshal([]byte(*raw), &looks); err != nil {
					return err
				}
			}
			if in.Look.Color != nil && (*in.Look.Color < 0 || *in.Look.Color > 7) {
				return errors.New("invalid workspace colour")
			}
			if names == nil {
				names = map[string]string{}
			}
			if looks == nil {
				looks = map[string]json.RawMessage{}
			}
			id := r.PathValue("id")
			if name := strings.TrimSpace(in.Name); name != "" {
				names[id] = name
			} else {
				delete(names, id)
			}
			if in.Look.Color != nil || in.Look.Pic != "" {
				looks[id], _ = json.Marshal(in.Look)
			} else {
				delete(looks, id)
			}
			rawNames, _ := json.Marshal(names)
			rawLooks, _ := json.Marshal(looks)
			normal, err := b.normalizePreference("myco-app-hub-looks", string(rawLooks))
			if err != nil {
				return err
			}
			nameValue := string(rawNames)
			out.Values["myco-app-hub-names"] = &nameValue
			out.Values["myco-app-hub-looks"] = &normal
			return nil
		})
	})
	m.HandleFunc("POST /app/preferences/images", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, preferenceImageLimit))
		if err != nil {
			preferenceFailure(w, 413, "The picture is too large.")
			return
		}
		b.preferenceMu.Lock()
		defer b.preferenceMu.Unlock()
		u, err := b.keepPreferenceImage(data)
		if err != nil {
			status := 400
			var storage *preferenceStorageError
			if errors.As(err, &storage) {
				status = 500
			}
			preferenceFailure(w, status, "Could not save this picture. Use a PNG, JPEG, GIF or WebP image and retry.")
			return
		}
		writeJSON(w, 200, map[string]string{"url": u})
	})
	m.HandleFunc("GET /app/preferences/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !preferenceImageName.MatchString(name) || b.preferenceDir() == "" {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(b.preferenceDir(), "images", name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		ext := filepath.Ext(name)
		types := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
		w.Header().Set("Content-Type", types[ext])
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		http.ServeContent(w, r, name, st.ModTime(), f)
	})
}

// Serialize across windows and App processes, then atomically replace metadata.
// A failed read or write never turns a damaged profile into empty defaults.
func (b *Backend) updatePreferences(w http.ResponseWriter, mutate func(*api.LocalPreferences) error) {
	b.preferenceMu.Lock()
	defer b.preferenceMu.Unlock()
	dir := b.preferenceDir()
	if dir == "" || os.MkdirAll(dir, 0700) != nil {
		preferenceFailure(w, 500, "Could not open local preferences.")
		return
	}
	unlock, err := lockPageOrigins(filepath.Join(dir, "preferences.lock"))
	if err != nil {
		preferenceFailure(w, 503, "Local preferences are busy. Please retry.")
		return
	}
	defer unlock()
	out, err := b.readPreferences()
	if err != nil {
		preferenceFailure(w, 500, "Could not read local preferences. No saved data was changed.")
		return
	}
	if err = mutate(&out); err != nil {
		var storage *preferenceStorageError
		if errors.As(err, &storage) {
			preferenceFailure(w, 500, "Could not save the picture. Your previous settings were preserved.")
			return
		}
		preferenceFailure(w, 400, "The preference or picture is invalid. Your previous settings were preserved.")
		return
	}
	data, err := json.Marshal(out)
	if err != nil || len(data) > preferencesLimit {
		preferenceFailure(w, 413, "Local preferences are too large.")
		return
	}
	if err = preferenceAtomic(filepath.Join(dir, "preferences.json"), data); err != nil {
		preferenceFailure(w, 500, "Could not save local preferences. Your previous settings were preserved.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

type preferenceStorageError struct{ error }

func (e *preferenceStorageError) Unwrap() error { return e.error }

var preferenceImageName = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|gif|webp)$`)

const preferenceImageURL = "/app/preferences/images/"

func (b *Backend) keepPreferenceImage(data []byte) (string, error) {
	if b.preferenceDir() == "" || len(data) == 0 || len(data) > preferenceImageLimit {
		return "", errors.New("invalid picture size or profile")
	}
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[http.DetectContentType(data)]
	if ext == "" {
		return "", errors.New("unsupported picture type")
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	path := filepath.Join(b.preferenceDir(), "images", name)
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return preferenceImageURL + name, nil
	}
	if err := preferenceAtomic(path, data); err != nil {
		return "", &preferenceStorageError{err}
	}
	return preferenceImageURL + name, nil
}
func (b *Backend) normalizePreference(key, value string) (string, error) {
	if key != "myco-app-hub-looks" {
		return value, nil
	}
	var looks map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &looks); err != nil {
		return "", err
	}
	for _, look := range looks {
		if raw, ok := look["pic"]; ok {
			var pic string
			if err := json.Unmarshal(raw, &pic); err != nil {
				return "", err
			}
			if pic == "" {
				continue
			}
			if strings.HasPrefix(pic, preferenceImageURL) {
				name := strings.TrimPrefix(pic, preferenceImageURL)
				if !preferenceImageName.MatchString(name) {
					return "", errors.New("invalid picture reference")
				}
				if _, err := os.Stat(filepath.Join(b.preferenceDir(), "images", name)); err != nil {
					return "", err
				}
				continue
			}
			header, encoded, ok := strings.Cut(pic, ",")
			if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
				return "", errors.New("invalid legacy picture")
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return "", err
			}
			url, err := b.keepPreferenceImage(data)
			if err != nil {
				return "", fmt.Errorf("picture: %w", err)
			}
			look["pic"], _ = json.Marshal(url)
		}
	}
	data, err := json.Marshal(looks)
	return string(data), err
}
