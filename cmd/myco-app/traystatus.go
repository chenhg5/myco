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
						Binding struct {
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
	for _, hub := range out.Hubs {
		if !hub.OK || hub.Overview == nil {
			st.Down = append(st.Down, hub.Name)
			continue
		}
		for _, a := range hub.Overview.Agents {
			if a.Agent.Binding.Harness == "machine" {
				continue
			}
			switch a.State {
			case "working":
				st.Working++
			case "pending":
				st.Pending++
			case "offline":
				st.Offline++
			default:
				st.Idle++
			}
		}
	}
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
	add(st.Working, "工作中", "working")
	add(st.Pending, "待处理", "pending")
	add(st.Idle, "空闲", "idle")
	add(st.Offline, "离线", "offline")
	if len(parts) == 0 {
		return zh.s("还没有节点", "No nodes yet")
	}
	return strings.Join(parts, " · ")
}
