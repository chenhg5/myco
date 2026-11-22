package main

import (
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

// appBadge sets the number on the App's icon (0: none).
type appBadge func(n int)

// newAppBadge: the Dock icon's badge.
func newAppBadge(app *application.App) appBadge {
	d := dock.New()
	app.RegisterService(application.NewService(d))
	return func(n int) {
		if n <= 0 {
			_ = d.RemoveBadge()
			return
		}
		_ = d.SetBadge(strconv.Itoa(n))
	}
}
