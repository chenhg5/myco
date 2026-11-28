package cli

import "testing"

func TestUpgradeReceiptFlagDoesNotWriteForDryRunsOrOtherCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"upgrade", []string{"upgrade", "--result-file", "receipt.json"}, "receipt.json"},
		{"global config", []string{"--config", "config.yaml", "upgrade", "--result-file=receipt.json"}, "receipt.json"},
		{"other command", []string{"agent", "update", "--result-file", "receipt.json"}, ""},
		{"dry run", []string{"upgrade", "--result-file", "receipt.json", "--dry-run"}, ""},
		{"explicit dry run", []string{"upgrade", "--result-file", "receipt.json", "--dry-run=true"}, ""},
		{"missing value", []string{"upgrade", "--result-file", "--dry-run"}, ""},
		{"literal argument", []string{"upgrade", "--", "--result-file", "receipt.json"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := upgradeReceiptFile(tc.args); got != tc.want {
				t.Fatalf("receipt=%q want %q", got, tc.want)
			}
		})
	}
}
