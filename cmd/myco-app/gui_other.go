//go:build !darwin

package main

import (
	"errors"
	"net/http"
)

func runGUI(http.Handler, func(string)) error {
	return errors.New("the app window is macOS only for now; run `myco-app --serve 127.0.0.1:0` and open the printed URL in a browser")
}
