package harness

import "encoding/json"

// ObserveProcess adds bounded, content-free milestones to a JSON CLI process.
// Session metadata and a model-origin event are deliberately separate: neither
// thread.started nor system/init proves that history restoration has finished.
func ObserveProcess(p *Process, req Request, cli string) {
	if req.OnTiming == nil {
		return
	}
	started := p.OnStarted
	ready := make(chan struct{})
	p.OnStarted = func() {
		defer close(ready)
		req.OnTiming("process_started")
		if started != nil {
			started()
		}
	}
	next := p.OnStdoutLine
	jsonSeen, sessionSeen, modelSeen := false, false, false
	p.OnStdoutLine = func(line []byte) {
		// Exec may start its stdout copier before cmd.Start returns. Preserve
		// milestone ordering without delaying any CLI input or network call.
		<-ready
		var v struct {
			Type     string          `json:"type"`
			Subtype  string          `json:"subtype"`
			APIError bool            `json:"isApiErrorMessage"`
			Error    json.RawMessage `json:"error"`
			Event    struct {
				Type string `json:"type"`
			} `json:"event"`
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if !modelSeen && json.Unmarshal(line, &v) == nil && v.Type != "" {
			if !jsonSeen {
				jsonSeen = true
				req.OnTiming("first_cli_event")
			}
			session := cli == "codex" && v.Type == "thread.started" || cli == "claude" && v.Type == "system" && v.Subtype == "init"
			if session && !sessionSeen {
				sessionSeen = true
				req.OnTiming("session_announced")
			}
			assistant := v.Type == "assistant" && !v.APIError && (len(v.Error) == 0 || string(v.Error) == "null")
			model := cli == "claude" && (assistant || v.Type == "stream_event" && v.Event.Type == "message_start")
			if cli == "codex" && (v.Type == "item.started" || v.Type == "item.updated" || v.Type == "item.completed") {
				switch v.Item.Type {
				case "agent_message", "reasoning", "command_execution", "mcp_tool_call", "file_change":
					model = true
				}
			}
			if model && !modelSeen {
				modelSeen = true
				req.OnTiming("first_model_event")
			}
		}
		if next != nil {
			next(line)
		}
	}
}
