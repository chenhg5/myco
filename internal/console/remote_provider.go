package console

import (
	"encoding/json"
	"net/http"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// Only this local App endpoint accepts plaintext keys. The subsequent Hub
// request contains the signed ciphertext; never proxy ManualProvider directly.
func (b *Backend) putRemoteAgentProvider(w http.ResponseWriter, r *http.Request, c *client.Client) {
	var in struct {
		Manual *ManualProvider `json:"manual"`
		// Workspace is one of the member's model providers (i115), with
		// the model the agent runs.
		Workspace string `json:"workspace,omitempty"`
		Model     string `json:"model,omitempty"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Workspace != "" {
		a, err := useProviderFor(r.Context(), c, r.PathValue("name"), in.Workspace, in.Model)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
		return
	}
	if in.Manual == nil {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "manual settings are required"})
		return
	}
	name := r.PathValue("name")
	a, err := c.GetAgent(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	cli := a.Binding.Harness
	if a.Runtime != nil && a.Runtime.CLI != "" {
		cli = a.Runtime.CLI
	}
	if err := in.Manual.Validate(cli); err != nil {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: err.Error()})
		return
	}
	raw, err := json.Marshal(in.Manual)
	if err != nil {
		writeErr(w, &client.Error{Status: 400, Code: "invalid_argument", Message: "invalid custom provider settings"})
		return
	}
	sealed, err := c.SealProvider(r.Context(), name, raw)
	if err != nil {
		writeErr(w, err)
		return
	}
	p := "custom"
	rs := api.RunSettingsPatch{Provider: &p, ProviderEncrypted: sealed}
	updated, err := c.PatchRunSettings(r.Context(), name, rs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
