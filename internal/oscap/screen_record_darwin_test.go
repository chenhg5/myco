package oscap

import (
	"reflect"
	"testing"
)

func TestScreenRecordCommand(t *testing.T) {
	a := screenRecordArgs{MaxSeconds: 20, Interactive: true, Microphone: true, Clicks: true}
	want := []string{"-v", "-V", "20", "-i", "-Jvideo", "-g", "-k", "/tmp/a movie.mov"}
	if got := screenRecordCommand(a, "/tmp/a movie.mov"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	a.Interactive = false
	a.Microphone = false
	a.Clicks = false
	want = []string{"-v", "-V", "20", "-m", "/tmp/a.mov"}
	if got := screenRecordCommand(a, "/tmp/a.mov"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
