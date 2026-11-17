// Package deploy holds the install scripts; this test runs install-hub.sh
// against a fake root with stand-ins for systemctl, ss, curl, caddy, nginx and
// the firewalls, so every way of installing can be checked without root,
// network or a real server.
package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeIP = "203.0.113.7"

// Stand-ins: each logs its arguments to $FAKE_LOG and answers from FAKE_*
// variables.
var stubs = map[string]string{
	"systemctl": `echo "systemctl $*" >> "$FAKE_LOG"
if [[ "$1" == is-active ]]; then
  unit="${@: -1}"
  for u in ${FAKE_ACTIVE-caddy nginx}; do [[ "$u" == "$unit" ]] && exit 0; done
  exit 3
fi
exit 0`,
	"ss": `p=0; port=""
for a in "$@"; do
  [[ "$a" == -* && "$a" == *p* ]] && p=1
  if [[ "$a" =~ sport\ =\ :([0-9]+) ]]; then port="${BASH_REMATCH[1]}"; fi
done
v="FAKE_PORT_$port"; proc="${!v:-}"
[[ -n "$proc" ]] || exit 0
if (( p )); then echo "LISTEN 0 511 0.0.0.0:$port 0.0.0.0:* users:((\"$proc\",pid=42,fd=6))"
else echo "LISTEN 0 511 0.0.0.0:$port 0.0.0.0:*"; fi`,
	"curl": `echo "curl $*" >> "$FAKE_LOG"
out=""; url=""; prev=""
for a in "$@"; do
  [[ "$prev" == -o ]] && out="$a"
  [[ "$a" == http* ]] && url="$a"
  prev="$a"
done
if [[ "$url" == *api.ipify.org* ]]; then printf '%s' "$FAKE_IP"; exit 0; fi
if [[ "$url" == */releases/* && -n "$out" ]]; then
  cat > "$out" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  "version "*) echo "myco v0.0.0-test" ;;
  "link create") echo '{"join_url": "https://hub.test/j/tok"}' ;;
  "console link") echo '{"link": "https://hub.test/c/app"}' ;;
esac
EOF
  exit 0
fi
if [[ -n "${FAKE_CURL_FAIL:-}" ]] && grep -Eq -- "$FAKE_CURL_FAIL" <<<"$*"; then exit 7; fi
echo '{"ok":true}'`,
	"caddy":      `echo "caddy $*" >> "$FAKE_LOG"; exit "${FAKE_CADDY_VALIDATE:-0}"`,
	"nginx":      `echo "nginx $*" >> "$FAKE_LOG"; if [[ "${FAKE_NGINX_T:-0}" == 1 ]]; then echo "nginx: [emerg] unknown directive" >&2; exit 1; fi; echo "nginx: configuration file test is successful" >&2`,
	"getent":     `[[ -n "${FAKE_DNS_NONE:-}" ]] && exit 2; echo "${FAKE_DNS_IP:-$FAKE_IP}  STREAM $2"`,
	"journalctl": `echo "(journal)"`,
	"sleep":      `exit 0`,
	"ufw": `echo "ufw $*" >> "$FAKE_LOG"
f="$FAKE_STATE/ufw"
case "$1" in
  status) echo "Status: ${FAKE_UFW:-inactive}"; [[ -f "$f" ]] && cat "$f" ;;
  allow) echo "$2 ALLOW Anywhere" >> "$f" ;;
esac
exit 0`,
	"firewall-cmd": `echo "firewall-cmd $*" >> "$FAKE_LOG"`,
}

// Real tools the script may use; nothing else on the machine is reachable.
var realTools = []string{"bash", "env", "sed", "awk", "grep", "head", "tail", "tr", "od", "sort", "mktemp",
	"install", "cat", "cp", "rm", "chmod", "ln", "dirname", "basename", "uname", "seq", "mkdir", "wc", "cut", "id", "readlink", "printf"}

type run struct {
	root, log string
	out       string
	code      int
}

func (r run) file(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.root, rel))
	if err != nil {
		t.Fatalf("read %s: %v\n--- output:\n%s", rel, err, r.out)
	}
	return string(b)
}

func (r run) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(r.root, rel))
	return err == nil
}

func (r run) calls(t *testing.T) string {
	b, _ := os.ReadFile(r.log)
	return string(b)
}

type env map[string]string

// install runs the script in root (made when empty) with the stand-ins.
func install(t *testing.T, root string, e env, args ...string) run {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install-hub.sh is a Linux script")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	if root == "" {
		root = t.TempDir()
	}
	bin := filepath.Join(root, ".bin")
	state := filepath.Join(root, ".state")
	for _, d := range []string{bin, state} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range realTools {
		if p, err := exec.LookPath(tool); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, tool))
		}
	}
	for name, body := range stubs {
		p := filepath.Join(bin, name)
		_ = os.Remove(p)
		if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(state, "calls.log")
	_ = os.Remove(log)
	cmd := exec.Command(bash, append([]string{"install-hub.sh"}, args...)...)
	cmd.Env = []string{
		"PATH=" + bin,
		"HOME=" + state,
		"MYCO_INSTALL_ROOT=" + root,
		"MYCO_INSTALL_TTY=" + filepath.Join(state, "no-tty"),
		"FAKE_LOG=" + log,
		"FAKE_STATE=" + state,
		"FAKE_IP=" + fakeIP,
	}
	for k, v := range e {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return run{root: root, log: log, out: out.String(), code: code}
}

func wantIn(t *testing.T, what, got string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(got, s) {
			t.Errorf("%s: missing %q\n--- got:\n%s", what, s, got)
		}
	}
}

func wantNotIn(t *testing.T, what, got string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(got, s) {
			t.Errorf("%s: unexpected %q\n--- got:\n%s", what, s, got)
		}
	}
}

func nginxConf(t *testing.T, root, includes string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "etc/nginx/conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/nginx/nginx.conf"), []byte("http {\n\tinclude "+includes+";\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sslip = "203-0-113-7.sslip.io"

func TestCaddyAuto(t *testing.T) {
	r := install(t, "", nil, "--domain", "auto")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	unit := r.file(t, "etc/systemd/system/myco-hub.service")
	wantIn(t, "unit", unit, "--addr 127.0.0.1:7777", "--public-url https://"+sslip, "EnvironmentFile=/etc/myco/hub.env")
	wantIn(t, "caddy site", r.file(t, "etc/caddy/myco.d/default.caddy"), sslip+" {", "reverse_proxy 127.0.0.1:7777", "read_timeout 120s")
	wantIn(t, "Caddyfile", r.file(t, "etc/caddy/Caddyfile"), "import /etc/caddy/myco.d/*.caddy")
	wantIn(t, "output", r.out, "==> HTTPS is up: https://"+sslip, "==> hub is healthy", "Hub URL:      https://"+sslip,
		"curl -fsSL https://hub.test/j/tok | bash", "https://hub.test/c/app")
	wantIn(t, "env", r.file(t, "etc/myco/hub.env"), "MYCO_ADMIN_TOKEN=", "MYCO_HUB=http://127.0.0.1:7777")

	// Re-running keeps the token.
	token := r.file(t, "etc/myco/hub.env")
	r2 := install(t, r.root, nil, "--domain", "auto")
	if r2.code != 0 || r2.file(t, "etc/myco/hub.env") != token {
		t.Fatalf("re-run changed the token or failed (exit %d)\n%s", r2.code, r2.out)
	}
}

func TestCaddyRefusesTakenPortAndSuggestsBehind(t *testing.T) {
	r := install(t, "", env{"FAKE_PORT_443": "nginx"}, "--domain", "hub.example.com")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "port 443 is already used", "--behind nginx --domain hub.example.com")
	if r.exists("etc/caddy/myco.d/default.caddy") {
		t.Error("wrote a Caddy site although 443 is taken")
	}
}

func TestBehindNginxWithoutCertificate(t *testing.T) {
	root := t.TempDir()
	nginxConf(t, root, "/etc/nginx/conf.d/*.conf")
	r := install(t, root, env{"FAKE_PORT_443": "nginx", "FAKE_PORT_80": "nginx"}, "--behind", "nginx", "--domain", "hub.example.com")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	site := r.file(t, "etc/nginx/conf.d/myco-default.conf")
	wantIn(t, "site", site, "server_name hub.example.com;", "listen 80;", "proxy_pass http://127.0.0.1:7777;",
		"proxy_read_timeout 360s;", "proxy_buffering off;", "client_max_body_size 64m;", "proxy_set_header Host $host;")
	wantNotIn(t, "site", site, "ssl_certificate")
	wantIn(t, "unit", r.file(t, "etc/systemd/system/myco-hub.service"), "--addr 127.0.0.1:7777", "--public-url http://hub.example.com")
	wantIn(t, "output", r.out, "your other sites are not touched", "certbot", "==> hub is healthy", "reachable at http://hub.example.com")
	wantIn(t, "calls", r.calls(t), "nginx -t", "systemctl reload nginx")
	if r.exists("etc/caddy/myco.d/default.caddy") {
		t.Error("B wrote a Caddy site")
	}
}

func TestBehindNginxUsesExistingCertificate(t *testing.T) {
	root := t.TempDir()
	nginxConf(t, root, "/etc/nginx/sites-enabled/*")
	if err := os.MkdirAll(filepath.Join(root, "etc/nginx/sites-enabled"), 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(root, "etc/letsencrypt/live/hub.example.com")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"fullchain.pem", "privkey.pem"} {
		_ = os.WriteFile(filepath.Join(live, f), []byte("x"), 0o600)
	}
	r := install(t, root, env{"FAKE_PORT_443": "nginx"}, "--behind", "nginx", "--domain", "hub.example.com")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	site := r.file(t, "etc/nginx/sites-available/myco-default.conf")
	wantIn(t, "site", site, "listen 443 ssl;", "ssl_certificate /etc/letsencrypt/live/hub.example.com/fullchain.pem;", "return 301 https://")
	if target, err := os.Readlink(filepath.Join(root, "etc/nginx/sites-enabled/myco-default.conf")); err != nil || !strings.HasSuffix(target, "sites-available/myco-default.conf") {
		t.Errorf("sites-enabled link: %q %v", target, err)
	}
	wantIn(t, "unit", r.file(t, "etc/systemd/system/myco-hub.service"), "--public-url https://hub.example.com")
}

func TestBehindNginxRejectedConfigIsPutBack(t *testing.T) {
	root := t.TempDir()
	nginxConf(t, root, "/etc/nginx/conf.d/*.conf")
	old := "# the version from before\n"
	p := filepath.Join(root, "etc/nginx/conf.d/myco-default.conf")
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r := install(t, root, env{"FAKE_NGINX_T": "1"}, "--behind", "nginx", "--domain", "hub.example.com")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "nginx rejected the configuration", "unknown directive")
	if got := r.file(t, "etc/nginx/conf.d/myco-default.conf"); got != old {
		t.Errorf("previous file not restored: %q", got)
	}
	wantNotIn(t, "calls", r.calls(t), "systemctl reload nginx")

	// Nothing there before: nothing left behind.
	root2 := t.TempDir()
	nginxConf(t, root2, "/etc/nginx/conf.d/*.conf")
	r2 := install(t, root2, env{"FAKE_NGINX_T": "1"}, "--behind", "nginx", "--domain", "hub.example.com")
	if r2.code != 1 || r2.exists("etc/nginx/conf.d/myco-default.conf") {
		t.Errorf("exit %d, file left: %v\n%s", r2.code, r2.exists("etc/nginx/conf.d/myco-default.conf"), r2.out)
	}
}

func TestBehindPanelPrintsSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "www/server/panel"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := install(t, root, env{"FAKE_PORT_443": "nginx"}, "--behind", "panel", "--domain", "hub.example.com")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "宝塔", "1Panel", "http://127.0.0.1:7777", "proxy_read_timeout 360s;", "client_max_body_size 64m;", "Let's Encrypt")
	wantIn(t, "unit", r.file(t, "etc/systemd/system/myco-hub.service"), "--addr 127.0.0.1:7777", "--public-url https://hub.example.com")
	if r.exists("etc/nginx") {
		t.Error("panel mode touched /etc/nginx")
	}
	wantNotIn(t, "calls", r.calls(t), "nginx -t", "reload nginx")
}

func TestPlainHTTP(t *testing.T) {
	r := install(t, "", env{"FAKE_PORT_8080": "myco", "FAKE_UFW": "active"}, "--http", "--port", "8080")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	wantIn(t, "unit", r.file(t, "etc/systemd/system/myco-hub.service"), "--addr 0.0.0.0:8080", "--public-url http://"+fakeIP+":8080")
	wantIn(t, "env", r.file(t, "etc/myco/hub.env"), "MYCO_HUB=http://127.0.0.1:8080")
	wantIn(t, "output", r.out, "Plain HTTP: tokens and messages cross the network unencrypted", "opening port 8080/tcp in ufw",
		"reachable at http://"+fakeIP+":8080", "security group")
	wantIn(t, "calls", r.calls(t), "ufw allow 8080/tcp")
}

func TestPlainHTTPNamesTheBlockedLayer(t *testing.T) {
	r := install(t, "", env{"FAKE_PORT_7777": "myco", "FAKE_CURL_FAIL": fakeIP + ":7777/v1/health"}, "--http")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "Layer: the cloud provider's firewall", "allow inbound TCP 7777", "install-hub.sh --check")
}

func TestSwitchingWayRemovesOurOwnSiteOnly(t *testing.T) {
	r := install(t, "", nil, "--domain", "auto")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	other := filepath.Join(r.root, "etc/caddy/myco.d/someone-else.caddy")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2 := install(t, r.root, env{"FAKE_PORT_7777": "myco"}, "--http")
	if r2.code != 0 {
		t.Fatalf("exit %d\n%s", r2.code, r2.out)
	}
	if r2.exists("etc/caddy/myco.d/default.caddy") {
		t.Error("our Caddy site is still there after switching to --http")
	}
	if !r2.exists("etc/caddy/myco.d/someone-else.caddy") {
		t.Error("removed a Caddy site that is not ours")
	}
}

func TestNoArgumentsWithoutTerminalListsTheWays(t *testing.T) {
	r := install(t, "", nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "--domain auto", "--behind nginx --domain", "--behind panel", "--http --port 7777", "--local")
	if r.exists("etc/systemd/system/myco-hub.service") {
		t.Error("installed without being told how")
	}
}

func TestQuestionsChooseBehindNginx(t *testing.T) {
	root := t.TempDir()
	nginxConf(t, root, "/etc/nginx/conf.d/*.conf")
	answers := filepath.Join(root, "answers")
	// Enter = the recommended way (2: behind nginx), then the domain, then yes.
	if err := os.WriteFile(answers, []byte("\nhub.example.com\nY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := install(t, root, env{"FAKE_PORT_443": "nginx", "MYCO_INSTALL_TTY": answers})
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	wantIn(t, "output", r.out, "port 443: nginx", "← 推荐 recommended", "Same without questions",
		"sudo bash install-hub.sh --behind nginx --domain hub.example.com")
	wantIn(t, "site", r.file(t, "etc/nginx/conf.d/myco-default.conf"), "server_name hub.example.com;")
}

func TestCheckOnly(t *testing.T) {
	r := install(t, "", nil, "--domain", "auto")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.out)
	}
	ok := install(t, r.root, nil, "--check")
	if ok.code != 0 {
		t.Fatalf("--check exit %d\n%s", ok.code, ok.out)
	}
	wantIn(t, "check", ok.out, "==> hub is healthy", "reachable at https://"+sslip)
	wantNotIn(t, "check", ok.out, "downloading")

	bad := install(t, r.root, env{"FAKE_CURL_FAIL": "resolve " + sslip + ":443"}, "--check")
	if bad.code != 1 {
		t.Fatalf("--check exit %d, want 1\n%s", bad.code, bad.out)
	}
	wantIn(t, "check", bad.out, "Layer: the certificate", "journalctl -u caddy")

	dns := install(t, r.root, env{"FAKE_DNS_IP": "198.51.100.9", "FAKE_CURL_FAIL": "^-fsS --max-time 8 https"}, "--check")
	wantIn(t, "check", dns.out, "resolves to 198.51.100.9", "Layer: DNS")
}
