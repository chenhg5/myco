package oscap

import (
	"context"
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
	"time"
)

func testEnv(t *testing.T) *Env {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return &Env{Home: home}
}

func call(t *testing.T, env *Env, name string, args any) map[string]any {
	t.Helper()
	v, err := callRaw(env, name, args)
	if err != nil {
		t.Fatalf("%s: %v (%s)", name, err, err.Code)
	}
	b, _ := json.Marshal(v)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: not an object: %s", name, b)
	}
	return m
}

func callRaw(env *Env, name string, args any) (any, *Error) {
	b, _ := json.Marshal(args)
	return Call(context.Background(), env, name, b)
}

func TestFiles(t *testing.T) {
	env := testEnv(t)
	// write (making the folders), read back, as text and as bytes
	w := call(t, env, "fs.write", map[string]any{"path": "~/a/b/note.md", "content": "你好\nmyco"})
	if w["type"] != "file" || w["size"].(float64) != 11 {
		t.Fatalf("write: %v", w)
	}
	r := call(t, env, "fs.read", map[string]any{"path": "~/a/b/note.md"})
	if r["content"] != "你好\nmyco" || r["encoding"] != "utf8" || r["eof"] != true {
		t.Fatalf("read: %v", r)
	}
	r = call(t, env, "fs.read", map[string]any{"path": "~/a/b/note.md", "offset": 7, "length": 2, "encoding": "base64"})
	if b, _ := base64.StdEncoding.DecodeString(r["content"].(string)); string(b) != "my" || r["eof"] != false {
		t.Fatalf("read a part: %v", r)
	}
	call(t, env, "fs.write", map[string]any{"path": "~/a/b/note.md", "content": "!", "append": true})
	if r = call(t, env, "fs.read", map[string]any{"path": "~/a/b/note.md"}); r["content"] != "你好\nmyco!" {
		t.Fatalf("append: %v", r)
	}
	bin := []byte{0xff, 0x00, 0xfe}
	call(t, env, "fs.write", map[string]any{"path": "~/bin.dat", "content": base64.StdEncoding.EncodeToString(bin), "encoding": "base64"})
	if r = call(t, env, "fs.read", map[string]any{"path": "~/bin.dat"}); r["encoding"] != "base64" {
		t.Fatalf("binary comes back as base64: %v", r)
	}
	if _, err := callRaw(env, "fs.write", map[string]any{"path": "~/bin.dat", "content": "x", "exclusive": true}); err == nil || err.Code != CodeExists {
		t.Fatalf("exclusive write over a file: %v", err)
	}
	if _, err := callRaw(env, "fs.read", map[string]any{"path": "relative.txt"}); err == nil || err.Code != CodeInvalid {
		t.Fatalf("a relative path: %v", err)
	}
	if _, err := callRaw(env, "fs.read", map[string]any{"path": "~/nope"}); err == nil || err.Code != CodeNotFound {
		t.Fatalf("a missing file: %v", err)
	}

	// list: hidden ones only when asked
	call(t, env, "fs.write", map[string]any{"path": "~/a/.hidden", "content": ""})
	l := call(t, env, "fs.list", map[string]any{"path": "~/a"})
	if n := len(l["entries"].([]any)); n != 1 {
		t.Fatalf("list without hidden: %v", l)
	}
	if l = call(t, env, "fs.list", map[string]any{"path": "~/a", "hidden": true}); len(l["entries"].([]any)) != 2 {
		t.Fatalf("list with hidden: %v", l)
	}

	// copy a folder, move it, delete it
	call(t, env, "fs.copy", map[string]any{"from": "~/a", "to": "~/c"})
	if r = call(t, env, "fs.read", map[string]any{"path": "~/c/b/note.md"}); r["content"] != "你好\nmyco!" {
		t.Fatalf("copy: %v", r)
	}
	if _, err := callRaw(env, "fs.copy", map[string]any{"from": "~/a", "to": "~/c"}); err == nil || err.Code != CodeExists {
		t.Fatalf("copy over: %v", err)
	}
	if _, err := callRaw(env, "fs.move", map[string]any{"from": "~/a", "to": "~/a/inside"}); err == nil {
		t.Fatal("moved a folder into itself")
	}
	call(t, env, "fs.move", map[string]any{"from": "~/c", "to": "~/d/e"})
	if _, err := os.Stat(filepath.Join(env.Home, "d", "e", "b", "note.md")); err != nil {
		t.Fatalf("move: %v", err)
	}
	d := call(t, env, "fs.delete", map[string]any{"paths": []string{"~/d", "~"}, "recursive": true})
	if len(d["done"].([]any)) != 1 || len(d["failed"].([]any)) != 1 {
		t.Fatalf("delete (the home folder refused): %v", d)
	}
	if _, err := os.Stat(filepath.Join(env.Home, "d")); !os.IsNotExist(err) {
		t.Fatalf("not deleted: %v", err)
	}

	p := call(t, env, "fs.paths", nil)
	if p["home"] != env.Home || p["downloads"] == "" {
		t.Fatalf("paths: %v", p)
	}
}

func TestTrashLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the freedesktop trash")
	}
	env := testEnv(t)
	call(t, env, "fs.write", map[string]any{"path": "~/x/old.log", "content": "1"})
	call(t, env, "fs.write", map[string]any{"path": "~/y/old.log", "content": "2"})
	r := call(t, env, "fs.trash", map[string]any{"paths": []string{"~/x/old.log", "~/y/old.log", "~/missing"}})
	if len(r["done"].([]any)) != 2 || len(r["failed"].([]any)) != 1 {
		t.Fatalf("trash: %v", r)
	}
	trash := filepath.Join(env.Home, ".local", "share", "Trash")
	for _, f := range []string{"files/old.log", "files/old.2.log", "info/old.log.trashinfo", "info/old.2.log.trashinfo"} {
		if _, err := os.Stat(filepath.Join(trash, f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	info, _ := os.ReadFile(filepath.Join(trash, "info", "old.log.trashinfo"))
	if !strings.Contains(string(info), "Path="+filepath.Join(env.Home, "x", "old.log")) || !strings.Contains(string(info), "DeletionDate=") {
		t.Fatalf("trashinfo: %s", info)
	}
}

func TestScan(t *testing.T) {
	env := testEnv(t)
	for i, size := range []int{10, 3000, 200, 50000} {
		dir := []string{"small", "big", "small", "big/deeper"}[i]
		call(t, env, "fs.write", map[string]any{"path": fmt.Sprintf("~/s/%s/f%d", dir, i), "content": strings.Repeat("x", size)})
	}
	js := &Jobs{Env: env}
	j, err := js.Start("fs.scan", json.RawMessage(`{"path":"~/s","depth":1,"top":2}`))
	if err != nil {
		t.Fatal(err)
	}
	b := wait(t, js, j.ID)
	if b.Error != nil {
		t.Fatal(b.Error)
	}
	out, _ := json.Marshal(b.Value)
	var v struct {
		Roots   []Node    `json:"roots"`
		Largest []BigFile `json:"largest"`
		Files   int       `json:"files"`
	}
	_ = json.Unmarshal(out, &v)
	if v.Files != 4 || len(v.Roots) != 1 || v.Roots[0].Size != 53210 {
		t.Fatalf("totals: %s", out)
	}
	kids := v.Roots[0].Children
	if len(kids) != 2 || kids[0].Name != "big" || kids[0].Size != 53000 || kids[0].Children != nil {
		t.Fatalf("children, largest first, one level: %s", out)
	}
	if len(v.Largest) != 2 || v.Largest[0].Size != 50000 || v.Largest[1].Size != 3000 {
		t.Fatalf("largest: %s", out)
	}
	if len(b.Events) == 0 || b.Events[len(b.Events)-1].Type != "progress" {
		t.Fatalf("progress events: %+v", b.Events)
	}
}

// wait reads a job's events until it ends.
func wait(t *testing.T, js *Jobs, id string) Batch {
	t.Helper()
	var all Batch
	next := 0
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		b, err := js.Read(context.Background(), id, next, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		all.Events = append(all.Events, b.Events...)
		next = b.Next
		if b.Done {
			all.Done, all.Value, all.Error = true, b.Value, b.Error
			return all
		}
	}
	t.Fatal("the job did not end")
	return all
}

func TestExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	env := testEnv(t)
	r := call(t, env, "exec.run", map[string]any{"shell": "echo $GREETING; pwd; echo oops >&2; exit 3", "env": map[string]string{"GREETING": "hi 你好"}})
	if r["code"].(float64) != 3 || r["stdout"] != "hi 你好\n"+env.Home+"\n" || r["stderr"] != "oops\n" {
		t.Fatalf("run: %v", r)
	}
	r = call(t, env, "exec.run", map[string]any{"argv": []string{"cat"}, "stdin": "piped"})
	if r["stdout"] != "piped" || r["code"].(float64) != 0 {
		t.Fatalf("stdin: %v", r)
	}
	r = call(t, env, "exec.run", map[string]any{"shell": "sleep 5", "timeout_ms": 200})
	if r["timed_out"] != true {
		t.Fatalf("timeout: %v", r)
	}
	if _, err := callRaw(env, "exec.run", map[string]any{"argv": []string{"no-such-program-myco"}}); err == nil || err.Code != CodeNotFound {
		t.Fatalf("missing program: %v", err)
	}
	r = call(t, env, "exec.run", map[string]any{"shell": "head -c 3000 /dev/zero | tr '\\0' x", "max_output": 100})
	if len(r["stdout"].(string)) != 100 || r["truncated"] != true {
		t.Fatalf("max output: %v", r)
	}

	// spawn: output as it comes, input written to it, then closed
	js := &Jobs{Env: env}
	j, err := js.Start("exec.spawn", json.RawMessage(`{"shell":"while read l; do echo got:$l; done; echo bye"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := js.Input(j.ID, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := js.Input(j.ID, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	_ = js.Input(j.ID, nil)
	b := wait(t, js, j.ID)
	var out strings.Builder
	for _, e := range b.Events {
		if e.Type == "stdout" {
			out.WriteString(e.Data.(string))
		}
	}
	if out.String() != "got:one\ngot:two\nbye\n" || b.Error != nil {
		t.Fatalf("spawn: %q %v", out.String(), b.Error)
	}

	// cancel stops it, and what it started
	j, _ = js.Start("exec.spawn", json.RawMessage(`{"shell":"sleep 30 & sleep 30"}`))
	time.Sleep(200 * time.Millisecond)
	_ = js.Cancel(j.ID)
	if b = wait(t, js, j.ID); b.Error == nil || b.Error.Code != CodeCancelled {
		t.Fatalf("cancel: %+v", b.Error)
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"method":%q,"auth":%q}`, r.Method, r.Header.Get("Authorization"))
	}))
	defer srv.Close()
	env := testEnv(t)
	r := call(t, env, "net.fetch", map[string]any{"url": srv.URL, "method": "post", "headers": map[string]string{"Authorization": "Bearer x"}, "body": "{}"})
	if r["status"].(float64) != 200 || r["body"] != `{"method":"POST","auth":"Bearer x"}` || r["encoding"] != "utf8" {
		t.Fatalf("fetch: %v", r)
	}
	if r = call(t, env, "net.fetch", map[string]any{"url": srv.URL + "/png"}); r["encoding"] != "base64" {
		t.Fatalf("binary: %v", r)
	}
	if _, err := callRaw(env, "net.fetch", map[string]any{"url": "file:///etc/passwd"}); err == nil || err.Code != CodeInvalid {
		t.Fatalf("file url: %v", err)
	}
}

func TestSystem(t *testing.T) {
	env := testEnv(t)
	i := call(t, env, "system.info", nil)
	if i["os"] != runtime.GOOS || i["cpus"].(float64) < 1 || i["name"] == "" {
		t.Fatalf("info: %v", i)
	}
	if runtime.GOOS == "linux" && i["memory"].(float64) <= 0 {
		t.Fatalf("memory: %v", i)
	}
	v, err := callRaw(env, "system.disks", nil)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && len(v.([]Disk)) == 0 {
		t.Log("no disks found (a container?)")
	}
	ps, err := callRaw(env, "system.processes", nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ps.([]Process) {
		if p.PID == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Fatalf("this test's own process is not in the list")
	}
	if _, err := callRaw(env, "process.kill", map[string]any{"pid": os.Getpid()}); err == nil {
		t.Fatal("killed itself")
	}
}

func TestList(t *testing.T) {
	l := List(&Env{})
	names := map[string]Info{}
	for _, i := range l {
		names[i.Name] = i
	}
	for _, n := range []string{"system.info", "fs.scan", "exec.spawn", "screen.capture", "net.fetch", "fs.trash"} {
		if _, ok := names[n]; !ok {
			t.Errorf("%s missing", n)
		}
	}
	if !names["fs.scan"].Job || names["fs.read"].Job {
		t.Error("jobs marked wrong")
	}
	if _, err := Call(context.Background(), &Env{}, "fs.scan", nil); err == nil {
		t.Error("a job was called as a single call")
	}
	if _, err := Call(context.Background(), &Env{}, "nope", nil); err == nil || err.Code != CodeInvalid {
		t.Errorf("unknown: %v", err)
	}
}
