package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// trayStatus counts agents across all hubs (machine nodes left out).
type trayStatus struct {
	Working, Pending, Idle, Offline int
	Down                            []string // hubs the app cannot reach
	Hubs                            int
	// Busy are the nodes at work (working first, then those with tasks
	// waiting), for the menu.
	Busy []trayNode
}

// trayNode is a busy node as the menu shows it.
type trayNode struct {
	Name    string // display name, else name
	Status  string // its status line ("" if none)
	Working bool   // false: tasks waiting
}

// readTrayStatus asks the app's own backend (in-process, no network) for
// the overview of every hub.
func readTrayStatus(h http.Handler) trayStatus {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/overview", nil))
	var out struct {
		Hubs []struct {
			Name     string `json:"name"`
			OK       bool   `json:"ok"`
			Overview *struct {
				Agents []struct {
					Agent struct {
						Name        string `json:"name"`
						DisplayName string `json:"display_name"`
						StatusText  string `json:"status_text"`
						Binding     struct {
							Harness string `json:"harness"`
						} `json:"binding"`
					} `json:"agent"`
					State string `json:"state"`
				} `json:"agents"`
			} `json:"overview"`
		} `json:"hubs"`
	}
	var st trayStatus
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		return st
	}
	st.Hubs = len(out.Hubs)
	var waiting []trayNode
	for _, hub := range out.Hubs {
		if !hub.OK || hub.Overview == nil {
			st.Down = append(st.Down, hub.Name)
			continue
		}
		for _, a := range hub.Overview.Agents {
			if a.Agent.Binding.Harness == "machine" {
				continue
			}
			node := trayNode{Name: a.Agent.DisplayName, Status: strings.TrimSpace(a.Agent.StatusText)}
			if node.Name == "" {
				node.Name = a.Agent.Name
			}
			switch a.State {
			case "working":
				st.Working++
				node.Working = true
				st.Busy = append(st.Busy, node)
			case "pending":
				st.Pending++
				waiting = append(waiting, node)
			case "offline":
				st.Offline++
			default:
				st.Idle++
			}
		}
	}
	st.Busy = append(st.Busy, waiting...)
	return st
}

// trayText picks the Chinese or English text.
type trayText bool

func (zh trayText) s(cn, en string) string {
	if zh {
		return cn
	}
	return en
}

// summary is the first line of the menu, e.g. "2 个工作中 · 1 个离线".
func (zh trayText) summary(st trayStatus) string {
	if st.Hubs == 0 {
		return zh.s("还没有连接 Hub", "No hub connected yet")
	}
	var parts []string
	add := func(n int, cn, en string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf(zh.s("%d 个"+cn, "%d "+en), n))
		}
	}
	add(st.Working, "在干活", "working")
	add(st.Pending, "有活待接", "with tasks waiting")
	add(st.Idle, "空闲", "idle")
	add(st.Offline, "离线", "offline")
	if len(parts) == 0 {
		return zh.s("还没有节点", "No nodes yet")
	}
	return strings.Join(parts, " · ")
}

// nodeLine is a busy node's menu line: "⚡ PM (Codex) — reviewing the branch".
func (zh trayText) nodeLine(n trayNode) string {
	icon, status := "⚡", n.Status
	if !n.Working {
		icon = "📬"
		if status == "" {
			status = zh.s("有任务在排队", "tasks waiting")
		}
	}
	line := icon + " " + clip(n.Name, 24)
	if status != "" {
		line += " — " + clip(strings.Join(strings.Fields(status), " "), 36)
	}
	return line
}

// clip shortens s to n characters with an ellipsis.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// wakeLabel is the menu item that restarts this computer's runner.
func (zh trayText) wakeLabel() string {
	return zh.s("唤醒本机节点", "Wake this computer's nodes")
}

// downLine names the hubs the app cannot reach.
func (zh trayText) downLine(down []string) string {
	return "⚠️ " + zh.s("连不上 ", "Can't reach ") + clip(strings.Join(down, zh.s("、", ", ")), 40)
}

// readTrayUpdate is the newer release the app can update to ("" if none,
// or when it cannot tell).
func readTrayUpdate(h http.Handler) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/update", nil))
	var u struct {
		Latest    string `json:"latest"`
		Available bool   `json:"available"`
		Error     string `json:"error"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &u) != nil || !u.Available || u.Error != "" {
		return ""
	}
	return u.Latest
}

// updateLine is the menu item for a newer release.
func (zh trayText) updateLine(latest string) string {
	return "⬆️ " + zh.s("有新版本 "+latest+"，点此更新", "myco "+latest+" is out — update")
}
