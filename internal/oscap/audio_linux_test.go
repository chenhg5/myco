package oscap

import "testing"

func TestSinkInputs(t *testing.T) {
	s := `Sink Input #42
	Driver: protocol-native.c
	Corked: no
	Properties:
		application.name = "zoom"
		application.process.binary = "zoom"
Sink Input #43
	Corked: yes
	Properties:
		application.name = "Firefox"
`
	got := sinkInputs(s)
	if len(got) != 2 || got[0].ID != "app:42" || got[0].Name != "zoom" || !got[0].Playing || got[1].Playing || got[1].Name != "Firefox" {
		t.Fatalf("%+v", got)
	}
}
