package cmdkit

import "testing"

func TestGlobalFlagsBeforeSubcommand(t *testing.T) {
	leaf := &Command{Name: "set", Run: func(*Invocation) error { return nil },
		Flags: []Flag{{Name: "alias", Kind: String}}}
	root := (&Command{Name: "myco", Flags: []Flag{
		{Name: "hub", Kind: String, Persistent: true},
		{Name: "verbose", Kind: Bool, Persistent: true},
	}}).Add((&Command{Name: "agent"}).Add((&Command{Name: "alias"}).Add(leaf)))

	cases := [][]string{
		{"--hub", "http://h", "--verbose", "agent", "alias", "set", "--alias", "x"},
		{"--hub=http://h", "agent", "--verbose", "alias", "set", "--alias", "x"},
		{"agent", "alias", "set", "--alias", "x", "--hub", "http://h", "--verbose"},
	}
	for _, args := range cases {
		cmd, vals, _, err := Resolve(root, args)
		if err != nil || cmd != leaf || vals["hub"][0] != "http://h" || vals["alias"][0] != "x" || vals["verbose"][0] != "true" {
			t.Errorf("%v: cmd=%v vals=%v err=%v", args, cmd.Name, vals, err)
		}
	}
	// A later flag overrides an earlier one.
	_, vals, _, _ := Resolve(root, []string{"--hub", "a", "agent", "alias", "set", "--hub", "b"})
	if vals["hub"][0] != "b" {
		t.Errorf("override: %v", vals)
	}
	// Leaf-only flags before the command are still an error.
	if _, _, _, err := Resolve(root, []string{"--alias", "x", "agent", "alias", "set"}); err == nil {
		t.Error("leaf flag before the command must fail")
	}
}
