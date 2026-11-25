package api

// BrowserActionPermission is the fixed v1 permission boundary shared by the
// Hub and desktop host. A component's advertised capabilities cannot weaken it.
func BrowserActionPermission(name string) string {
	switch name {
	case "browser.profiles.list", "browser.run.status":
		return "browser.view"
	case "browser.script.run", "browser.run.resume", "browser.run.cancel":
		return "browser.run"
	case "browser.page.open", "browser.page.snapshot", "browser.page.act", "browser.page.extract", "browser.page.screenshot", "browser.page.close":
		return BrowserControlPermission
	default:
		return ""
	}
}
