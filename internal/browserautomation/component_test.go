package browserautomation

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The test executable can act as an actual child process. This exercises
// handshake/EOF/environment boundaries on native Windows as well as macOS.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == program("myco-browser") {
		if len(os.Args) == 2 && os.Args[1] == "--cleanup-browser-groups" {
			var cfg struct {
				Dir      string `json:"data_dir"`
				OwnerPID int    `json:"owner_pid"`
			}
			dec := json.NewDecoder(os.Stdin)
			dec.DisallowUnknownFields()
			if dec.Decode(&cfg) != nil || !filepath.IsAbs(cfg.Dir) || os.Getenv("BROWSER_FIXTURE_PRIVATE") != "" {
				os.Exit(6)
			}
			raw, _ := json.Marshal(cfg)
			if os.WriteFile(filepath.Join(cfg.Dir, fmt.Sprintf("cleanup-%d.json", cfg.OwnerPID)), raw, 0600) != nil {
				os.Exit(7)
			}
			os.Exit(0)
		}

		var cfg struct {
			Secret string `json:"secret"`
			Dir    string `json:"data_dir"`
		}
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadBytes('\n')
		if err != nil || json.Unmarshal(line, &cfg) != nil || len(cfg.Secret) < 32 || !filepath.IsAbs(cfg.Dir) {
			os.Exit(3)
		}
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			os.Exit(4)
		}
		fixtureVersion := filepath.Base(filepath.Dir(os.Args[0]))
		if !CompatibleComponentVersion(fixtureVersion) {
			os.Exit(5)
		}
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+cfg.Secret || r.Header.Get("X-Myco-Scope") == "" {
				http.Error(w, "denied", 401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"protocol": Protocol, "version": fixtureVersion, "environment_clean": os.Getenv("BROWSER_FIXTURE_PRIVATE") == "", "display": os.Getenv("DISPLAY"), "wayland": os.Getenv("WAYLAND_DISPLAY"), "xauthority": os.Getenv("XAUTHORITY"), "runtime_dir": os.Getenv("XDG_RUNTIME_DIR")})
		})}
		groupProtocol := 1
		if _, err := os.Stat(filepath.Join(cfg.Dir, "legacy-group-protocol")); err == nil {
			groupProtocol = 0
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"address": listener.Addr().String(), "protocol": Protocol, "process_groups": groupProtocol})
		go func() { io.Copy(io.Discard, reader); server.Close() }()
		server.Serve(listener)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(data))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestArchiveBoundary(t *testing.T) {
	for _, files := range []map[string]string{{"../escape": "bad"}, {"x\\escape": "bad"}, {"LICENSE": "missing engine"}, {program("myco-browser"): "engine", "script.sh": "unexpected"}} {
		if err := extractComponent(archive(t, files), t.TempDir()); err == nil {
			t.Fatal("unsafe or incomplete archive accepted")
		}
	}
	if err := extractComponent(archive(t, map[string]string{program("myco-browser"): "fixture", "LICENSE.txt": "MIT"}), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
func TestAbsentDoesNotCreateFilesOrInstall(t *testing.T) {
	c := &Component{Home: t.TempDir()}
	s := c.Status()
	if s.Installed || s.Running {
		t.Fatal(s)
	}
	if _, err := os.Stat(filepath.Join(c.Home, ".myco")); !os.IsNotExist(err) {
		t.Fatal("status wrote private storage")
	}
	if c.EnsureRunning(context.Background()) == nil {
		t.Fatal("started an absent component")
	}
}
func TestChildHandshakeAndStop(t *testing.T) {
	c := &Component{Home: t.TempDir()}
	if err := os.MkdirAll(c.dir(), 0700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(c.dir(), program("myco-browser")), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER_FIXTURE_PRIVATE", "never forward")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer c.Stop()
	if err = c.EnsureRunning(ctx); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Protocol int  `json:"protocol"`
		Clean    bool `json:"environment_clean"`
	}
	if err = c.Call(ctx, "page-a/instance-a", "GET", "/status", nil, &status); err != nil || status.Protocol != Protocol || !status.Clean {
		t.Fatalf("child status/environment: %+v %v", status, err)
	}
	c.Stop()
	if c.Status().Running {
		t.Fatal("stdin EOF did not stop child")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestInstallChecksDigestAndKeepsPrivateProfiles(t *testing.T) {
	if !browserSupported() {
		t.Skip("component packages are Mac/Windows only")
	}
	for _, valid := range []bool{false, true} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			c := &Component{Home: t.TempDir()}
			name := "myco-browser_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
			data := archive(t, map[string]string{program("myco-browser"): "fixture", "LICENSE.txt": "MIT"})
			sum := sha256.Sum256(data)
			digest := hex.EncodeToString(sum[:])
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := data
				if strings.HasSuffix(r.URL.Path, "SHA256SUMS") {
					body = []byte(digest + "  " + name + "\n")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})}
			err := c.Install(context.Background())
			if valid && err != nil {
				t.Fatal(err)
			}
			if !valid && (err == nil || c.Status().Installed) {
				t.Fatal("bad digest installed")
			}
			if valid {
				profiles := filepath.Join(c.Home, ".myco", "browser", "profiles", "p_fixture")
				os.MkdirAll(profiles, 0700)
				os.WriteFile(filepath.Join(profiles, "marker"), []byte("keep"), 0600)
				if err = c.Remove(); err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(filepath.Join(profiles, "marker")); err != nil {
					t.Fatal("uninstall deleted sign-ins")
				}
			}
		})
	}
}

// Exercise the real child handshake, download and process replacement on every
// CI platform; Install's public platform gate remains Mac/Windows-only.
func TestUpdateCompatibleComponent(t *testing.T) {
	for _, oldVersion := range []string{"0.1.0", "0.1.1", "0.2.0", "0.3.0", "0.3.1"} {
		for _, valid := range []bool{false, true} {
			t.Run(oldVersion+"/"+fmt.Sprint(valid), func(t *testing.T) {
				c := &Component{Home: t.TempDir()}
				defer c.Stop()
				oldDir := filepath.Join(c.Home, ".myco", "components", "browser", oldVersion)
				if err := os.MkdirAll(oldDir, 0700); err != nil {
					t.Fatal(err)
				}
				binary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(binary)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(oldDir, program("myco-browser")), data, 0700); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err = c.EnsureRunning(ctx); err != nil {
					t.Fatal(err)
				}
				before := c.Status()
				if before.Version != oldVersion || !before.UpdateAvailable || !before.Running {
					t.Fatal(before)
				}
				c.mu.Lock()
				oldDone := c.done
				c.mu.Unlock()
				name := "myco-browser_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
				payload := archive(t, map[string]string{program("myco-browser"): string(data)})
				sum := sha256.Sum256(payload)
				digest := hex.EncodeToString(sum[:])
				if !valid {
					digest = strings.Repeat("0", 64)
				}
				requests := 0
				c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					requests++
					if !strings.HasPrefix(r.URL.String(), ComponentRelease+"/") || !strings.Contains(r.URL.Path, "/browser-v0.6.1/") {
						t.Errorf("wrong release: %s", r.URL)
					}
					if !c.Status().Running {
						t.Error("stopped old child before verifying download")
					}
					body := payload
					if strings.HasSuffix(r.URL.Path, "SHA256SUMS") {
						body = []byte(digest + "  " + name + "\n")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
				})}
				if err = c.install(ctx); !valid {
					if err == nil || !c.Status().Running || c.Status().Version != oldVersion {
						t.Fatalf("failed update lost old installation: %v %+v", err, c.Status())
					}
					select {
					case <-oldDone:
						t.Fatal("failed update stopped old process")
					default:
					}
					return
				} else if err != nil {
					t.Fatal(err)
				}
				if requests != 2 {
					t.Fatalf("downloads: %d", requests)
				}
				select {
				case <-oldDone:
				default:
					t.Fatal("upgrade did not stop old child")
				}
				after := c.Status()
				if after.Version != ComponentVersion || after.UpdateAvailable || after.Running {
					t.Fatal(after)
				}
				if err = c.EnsureRunning(ctx); err != nil {
					t.Fatal(err)
				}
				var actual struct {
					Version string `json:"version"`
				}
				if err = c.Call(ctx, "upgrade-fixture", "GET", "/status", nil, &actual); err != nil || actual.Version != ComponentVersion {
					t.Fatalf("new process: %+v %v", actual, err)
				}
			})
		}
	}
}

func TestCompatibleComponentVersions(t *testing.T) {
	for version, want := range map[string]bool{"0.1.0": true, "0.1.1": true, "0.2.0": true, "0.3.0": true, "0.3.1": true, "0.4.0": true, "0.5.0": true, "0.6.0": true, "0.6.1": true, "0.1.2": false, "": false} {
		if CompatibleComponentVersion(version) != want {
			t.Errorf("version %q compatibility", version)
		}
	}
}

func TestPreviewComponentOnlyForPreviewBuilds(t *testing.T) {
	home := t.TempDir()
	preview := PreviewDir(home)
	if err := os.MkdirAll(preview, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(preview, program("myco-browser")), []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	release := &Component{Home: home}
	if dir, _, installed := release.installedComponent(); installed || dir == preview {
		t.Fatalf("release build used the preview component: %s %v", dir, installed)
	}
	c := &Component{Home: home, Preview: true}
	dir, version, installed := c.installedComponent()
	if !installed || dir != preview || version != ComponentVersion {
		t.Fatalf("preview build: %s %s %v", dir, version, installed)
	}
	if s := c.Status(); !s.Installed || s.UpdateAvailable || s.Version != ComponentVersion {
		t.Fatalf("preview status: %+v", s)
	}
	// Removing release installations leaves the preview component alone.
	if err := c.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(preview, program("myco-browser"))); err != nil {
		t.Fatalf("preview component removed: %v", err)
	}
}
