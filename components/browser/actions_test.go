package main

import "testing"

func TestKeyEvents(t *testing.T) {
	for _, key := range []string{"Enter", "Tab", "Backspace", "Control+a", "Meta+Shift+ArrowLeft", "Control++", "ArrowDown"} {
		down, up, err := keyEvents(key)
		if err != nil || down.Code == "" || down.WindowsVirtualKeyCode == 0 || up.Text != "" {
			t.Fatalf("%s: %#v %#v %v", key, down, up, err)
		}
	}
	down, _, _ := keyEvents("Control+a")
	if down.Text != "" || down.Modifiers != 2 {
		t.Fatal("shortcut inserted text")
	}
	down, _, _ = keyEvents("Enter")
	if down.Text != "\r" {
		t.Fatal("enter missing native text")
	}
	for _, key := range []string{"NoSuchKey", "Hyper+a", "", "Control+未知"} {
		if _, _, err := keyEvents(key); err == nil {
			t.Fatalf("accepted %s", key)
		}
	}
}
