package main

import (
	"testing"
	"unsafe"
)

func TestUnicodePacketsKeepSurrogatePairsAndReleaseEveryUnit(t *testing.T) {
	events := unicodeInputs("A中🙂")
	if len(events) != 8 || unsafe.Sizeof(events[0]) != 40 {
		t.Fatalf("invalid Windows INPUT layout or Unicode units")
	}
	want := []uint16{65, 0x4e2d, 0xd83d, 0xde42}
	for i, u := range want {
		if events[2*i].Scan != u || events[2*i+1].Scan != u || events[2*i].VK != 0 || events[2*i].Flags != 4 || events[2*i+1].Flags != 6 {
			t.Fatal("Unicode unit lost or not released")
		}
	}
}
