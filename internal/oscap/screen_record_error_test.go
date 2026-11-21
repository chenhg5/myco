package oscap

import (
	"errors"
	"strings"
	"testing"
)

func TestScreenCaptureFailureKeepsActualCause(t *testing.T) {
	err := screenCaptureFailure(errors.New("exit status 1"), "screencapture: video not valid with -i", "/private/movie.mov")
	if AsError(err).Code != CodeFailed || !strings.Contains(err.Error(), "video not valid with -i") {
		t.Fatal(err)
	}
	err = screenCaptureFailure(errors.New("exit status 1"), "cannot write /private/movie.mov", "/private/movie.mov")
	if strings.Contains(err.Error(), "/private/movie.mov") || !strings.Contains(err.Error(), "<recording>") {
		t.Fatal(err)
	}
	err = screenCaptureFailure(errors.New("exit status 1"), "", "/private/movie.mov")
	if AsError(err).Code != CodeCancelled {
		t.Fatal(err)
	}
}
