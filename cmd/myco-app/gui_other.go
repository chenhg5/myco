//go:build !darwin && !windows

package main

import (
	"errors"
	"net/http"
)

// guiShell is what /app/info reports for the window.
const guiShell = "browser"

func runGUI(http.Handler, func(string), func(func())) error {
	return errors.New("the app window is for macOS and Windows; run `myco-app --serve 127.0.0.1:0` and open the printed URL in a browser")
}
