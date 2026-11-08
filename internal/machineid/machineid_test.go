package machineid

import "testing"

func TestFingerprintIsStableHashedAndNotTheRawID(t *testing.T) {
	a, b := Fingerprint(), Fingerprint()
	if a != b || len(a) != 32 {
		t.Fatalf("fingerprint %q / %q", a, b)
	}
	if r := raw(); r == "" || a == r || a == hash("other") {
		t.Fatalf("fingerprint must be a hash of the raw id")
	}
	if Cloud("claude-cloud_default") == Cloud("other") {
		t.Fatal("cloud environments must differ")
	}
}
