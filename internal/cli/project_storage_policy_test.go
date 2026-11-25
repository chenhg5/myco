package cli

import (
	"context"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"testing"
)

func TestProjectStoragePolicyConfiguration(t *testing.T) {
	makePolicy := func(args []string, env map[string]string) (int, int64, error) {
		cmd, vals, rest, e := Resolve(hubCommands(), append([]string{"serve"}, args...))
		if e != nil {
			return 0, 0, e
		}
		in := NewInvocation(context.Background(), cmd, vals, rest, nil, nil, nil, func(k string) string { return env[k] })
		p, e := projectStoragePolicy(in)
		return p.FilesMax, p.BytesMax, e
	}
	f, b, e := makePolicy(nil, nil)
	if e != nil || f != 1000 || b != 2<<30 {
		t.Fatalf("default %d %d %v", f, b, e)
	}
	env := map[string]string{"MYCO_PROJECT_FILES_MAX": "2500", "MYCO_PROJECT_BYTES_MAX": "4294967296"}
	f, b, e = makePolicy(nil, env)
	if e != nil || f != 2500 || b != 4<<30 {
		t.Fatalf("env %d %d %v", f, b, e)
	}
	f, b, e = makePolicy([]string{"--project-files-max", "3000"}, env)
	if e != nil || f != 3000 || b != 4<<30 {
		t.Fatalf("flag precedence %d %d %v", f, b, e)
	}
	for _, args := range [][]string{{"--project-files-max", "10001"}, {"--project-bytes-max", "21474836481"}, {"--project-bytes-max", "oops"}, {"--disk-min-free-percent", "100"}, {"--disk-min-free-bytes", "-1"}} {
		if _, _, e = makePolicy(args, nil); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, key := range []string{"MYCO_PROJECT_FILES_MAX", "MYCO_PROJECT_BYTES_MAX", "MYCO_DISK_MIN_FREE_BYTES", "MYCO_DISK_MIN_FREE_PERCENT"} {
		if _, _, e = makePolicy(nil, map[string]string{key: "invalid"}); e == nil {
			t.Fatalf("accepted invalid %s", key)
		}
	}

}
