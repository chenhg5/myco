package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/uilocale"
)

// trayStatus counts agents across all hubs. Only agents: machine and shell
// nodes (a workspace running commands for an agent) are left out, as in the
// App's sidebar and the notch island.
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
						Kind        string `json:"kind"`
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
			if !trayCounts(a.Agent.Kind, a.Agent.Binding.Harness) {
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

// trayCounts says whether a node is an agent (the App's agentKind): the
// hub's kind when it gives one, else from the harness.
func trayCounts(kind, harness string) bool {
	if kind != "" {
		return kind == "agent"
	}
	switch harness {
	case "machine", "shell", "exec", "human":
		return false
	}
	return true
}

// trayLocale keeps the same language as the App's pages.
type trayLocale string

func trayText(chinese bool) trayLocale {
	if chinese {
		return "zh"
	}
	return "en"
}
func trayLanguage(language string) trayLocale {
	if api.UILanguage(language).Valid() {
		return trayLocale(language)
	}
	return "en"
}
func (language trayLocale) s(cn, en string) string { return uilocale.Text(string(language), cn, en) }

// summary is the first line of the menu, e.g. "2 个工作中 · 1 个离线".
func (zh trayLocale) summary(st trayStatus) string {
	if st.Hubs == 0 {
		return zh.s("还没有连接 Hub", "No hub connected yet")
	}
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf(label, n))
		}
	}
	add(st.Working, zh.s("%d 个在干活", "%d working"))
	add(st.Pending, zh.s("%d 个有活待接", "%d with tasks waiting"))
	add(st.Idle, zh.s("%d 个空闲", "%d idle"))
	add(st.Offline, zh.s("%d 个离线", "%d offline"))
	if len(parts) == 0 {
		return zh.s("还没有节点", "No nodes yet")
	}
	return strings.Join(parts, " · ")
}

// nodeLine is a busy node's menu line: "PM (Codex) — reviewing the branch",
// or "— tasks waiting" for one not at work yet.
func (zh trayLocale) nodeLine(n trayNode) string {
	status := n.Status
	if status == "" {
		status = zh.s("在干活", "working")
		if !n.Working {
			status = zh.s("有任务在排队", "tasks waiting")
		}
	}
	line := clip(n.Name, 24)
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
func (zh trayLocale) wakeLabel() string {
	return zh.s("唤醒本机节点", "Wake this computer's nodes")
}

// downLine names the hubs the app cannot reach.
func (zh trayLocale) downLine(down []string) string {
	return zh.s("连不上 ", "Can't reach ") + clip(strings.Join(down, zh.s("、", ", ")), 40)
}

// readUILang is the language the App's pages last said ("" if not yet).
func readUILang(h http.Handler) string {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/ui-lang", nil))
	var out struct {
		Lang string `json:"lang"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		return ""
	}
	return out.Lang
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
func (zh trayLocale) updateLine(latest string) string {
	return fmt.Sprintf(zh.s("有新版本 %s，点此更新", "myco %s is out: update"), latest)
}
