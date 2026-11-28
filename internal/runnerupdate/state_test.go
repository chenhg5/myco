package runnerupdate

import (
	"github.com/myco-ai/myco/internal/osutil"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReceiptIsPrivateAndRedactsDownloadCredentials(t *testing.T) {
	base := t.TempDir()
	message := SafeMessage("download https://user:secret@example.com/file?token=hidden: failed")
	if strings.Contains(message, "secret") || strings.Contains(message, "hidden") {
		t.Fatal("credentials retained")
	}
	if err := Write(base, "cli", Receipt{State: "failed", At: time.Now(), Error: message}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(Path(base, "cli")); err != nil || osutil.UnixPermissions && st.Mode().Perm() != 0600 {
		t.Fatalf("receipt permissions: %v %v", st, err)
	}
	var got Receipt
	if err := Read(base, "cli", &got); err != nil || got.Error != message {
		t.Fatalf("receipt: %+v %v", got, err)
	}
}
