// Package testsshd provides a loopback-only real OpenSSH server for integration tests.
package testsshd

import (
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func Start(t *testing.T) (homeDir, clientConfig string) { return StartWithCommand(t, "") }

// StartWithCommand uses a test-only forced command to isolate systemd/Docker
// mutations from the test machine. An empty command runs actual Linux reads.
func StartWithCommand(t *testing.T, forced string) (homeDir, clientConfig string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("isolated Linux sshd")
	}
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("OpenSSH server not installed")
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed")
	}
	home := t.TempDir()
	key := filepath.Join(home, "id")
	host := filepath.Join(home, "host")
	for _, f := range []string{key, host} {
		if b, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", f).CombinedOutput(); err != nil {
			t.Fatalf("fixture key: %v %s", err, b)
		}
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	pub, _ := os.ReadFile(host + ".pub")
	known := filepath.Join(home, "known_hosts")
	os.WriteFile(known, []byte("[127.0.0.1]:"+strconv.Itoa(port)+" "+string(pub)), 0600)
	config := filepath.Join(home, "sshd_config")
	text := "ListenAddress 127.0.0.1\nPort " + strconv.Itoa(port) + "\nHostKey " + host + "\nPidFile " + filepath.Join(home, "pid") + "\nAuthorizedKeysFile " + key + ".pub\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes no\nPermitRootLogin prohibit-password\nLogLevel ERROR\n"
	if forced != "" {
		text += "ForceCommand " + forced + "\n"
	}
	os.WriteFile(config, []byte(text), 0600)
	c := exec.Command(sshd, "-D", "-e", "-f", config)
	logfile, err := os.Create(filepath.Join(home, "sshd.log"))
	if err != nil {
		t.Fatal(err)
	}
	c.Stdout, c.Stderr = logfile, logfile
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait(); logfile.Close() })
	for end := time.Now().Add(3 * time.Second); ; {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(end) {
			b, _ := os.ReadFile(logfile.Name())
			t.Fatalf("sshd did not start: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	sshconfig := filepath.Join(home, "config")
	os.WriteFile(sshconfig, []byte("Host fixture\n HostName 127.0.0.1\n User "+u.Username+"\n Port "+strconv.Itoa(port)+"\n IdentityFile "+key+"\n IdentitiesOnly yes\n UserKnownHostsFile "+known+"\n GlobalKnownHostsFile /dev/null\n"), 0600)
	return home, sshconfig
}
