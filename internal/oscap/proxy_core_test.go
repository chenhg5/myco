package oscap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in verifies actual cores without changing system networking.
// Every process, config, secret and controller port is disposable.
func TestProxyRealCores(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"mihomo", "PROXY_TEST_CORE"}, {"sing-box", "PROXY_TEST_SING_BOX"}} {
		t.Run(engine.name, func(t *testing.T) {
			binary := os.Getenv(engine.env)
			if binary == "" {
				t.Skipf("set %s to the executable for this integration check", engine.env)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().String()
			listener.Close()
			dir := t.TempDir()
			var config string
			var argv []string
			selected := "REJECT"
			file := filepath.Join(dir, "config")
			if engine.name == "mihomo" {
				config = fmt.Sprintf("external-controller: %s\nsecret: integration-only\nmode: rule\nlog-level: silent\nproxies: []\nproxy-groups:\n  - name: Choose\n    type: select\n    proxies: [DIRECT, REJECT]\nrules: [\"MATCH,Choose\"]\n", address)
				argv = []string{"-d", dir, "-f", file}
			} else {
				selected = "Direct B"
				config = fmt.Sprintf(`{"log":{"disabled":true},"outbounds":[{"type":"direct","tag":"Direct A"},{"type":"direct","tag":"Direct B"},{"type":"selector","tag":"Choose","outbounds":["Direct A","Direct B"]}],"route":{"rules":[{"clash_mode":"global","outbound":"Direct B"},{"clash_mode":"direct","outbound":"Direct A"}],"final":"Choose"},"experimental":{"clash_api":{"external_controller":%q,"secret":"integration-only","default_mode":"rule"}}}`, address)
				argv = []string{"run", "-D", dir, "-c", file}
			}
			if err := os.WriteFile(file, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, argv...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); cmd.Wait() }()
			e := &Env{Secrets: &proxySecrets{data: map[string]string{}}, Scope: "real-core-test"}
			proxyCallTest(t, e, "proxy.profile", map[string]string{"action": "save", "endpoint": "http://" + address, "engine": engine.name, "secret": "integration-only"})
			var v any
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				v, err = func() (any, error) {
					value, e := proxyCallTest(t, e, "proxy.control", map[string]string{"action": "snapshot"})
					if e != nil {
						return nil, e
					}
					return value, nil
				}()
				if err == nil {
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if err != nil {
				t.Fatal("core did not start", err)
			}
			state := v.(map[string]any)
			if len(state["warnings"].([]string)) > 0 {
				t.Fatal(state)
			}
			for _, a := range []map[string]string{{"action": "select", "group": "Choose", "name": selected}, {"action": "mode", "mode": "global"}} {
				if _, err := proxyCallTest(t, e, "proxy.control", a); err != nil {
					t.Fatal(err)
				}
			}
			v, capErr := proxyCallTest(t, e, "proxy.control", map[string]string{"action": "snapshot"})
			if capErr != nil {
				t.Fatal(capErr)
			}
			raw, _ := json.Marshal(v)
			var result struct {
				Config  proxyConfig          `json:"config"`
				Proxies map[string]proxyNode `json:"proxies"`
			}
			if json.Unmarshal(raw, &result) != nil || result.Config.Mode != "global" || result.Proxies["Choose"].Now != selected {
				t.Fatalf("core did not apply changes: %s", raw)
			}
			t.Logf("real %s: %s; snapshot, selector and mode verified", engine.name, state["version"])
		})
	}
}
