package browserautomation

import "time"

func cleanupRecordedBrowserGroups(string, string, int) error { return nil }

func hasRecordedBrowserGroups(string) (bool, error) { return false, nil }

func componentStopGrace() time.Duration { return 3 * time.Second }
