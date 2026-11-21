package oscap

import (
	"reflect"
	"testing"
)

func TestScreenRecordCommand(t *testing.T) {
	a := screenRecordArgs{MaxSeconds: 20, Interactive: true, Microphone: true, Clicks: true}
	want := []string{"-V", "20", "-Jvideo", "-g", "-k", "/tmp/a movie.mov"}
	if got := screenRecordCommand(a, "/tmp/a movie.mov"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	a.Interactive = false
	a.Microphone = false
	a.Clicks = false
	want = []string{"-V", "20", "-v", "-m", "/tmp/a.mov"}
	if got := screenRecordCommand(a, "/tmp/a.mov"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	a.Region = &screenRecordRegion{X: 0, Y: 0, W: 16, H: 16}
	want = []string{"-V", "20", "-v", "-R", "0,0,16,16", "/tmp/a.mov"}
	if got := screenRecordCommand(a, "/tmp/a.mov"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
