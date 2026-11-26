//go:build !darwin

package quota

func codexKeychain(string) []byte { return nil }
