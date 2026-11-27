package harness

import (
	"reflect"
	"testing"
)

func TestWorkBuddyLinuxPackagePaths(t *testing.T) {
	// Official deb 5.11.6.44396970: data.tar.xz contains the first path at
	// mode 0755. The second layout is documented in the UOS/Kylin FAQ.
	want := []string{
		"/opt/WorkBuddy/resources/app.asar.unpacked/cli/bin/codebuddy",
		"/opt/apps/workbuddy/resources/app.asar.unpacked/cli/bin/codebuddy",
	}
	if got := WorkBuddyBinaries("/home/demo"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Linux package paths = %v, want %v", got, want)
	}
}
