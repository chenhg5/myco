//go:build windows

package sysproxy

import "golang.org/x/sys/windows/registry"

func readSystem() Setting {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return Setting{}
	}
	defer k.Close()
	enable, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	override, _, _ := k.GetStringValue("ProxyOverride")
	return parseWindows(enable, server, override)
}
