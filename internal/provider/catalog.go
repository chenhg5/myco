package provider

import (
	"net/url"
	"strings"
)

// Vendor is a model vendor whose API keys agents use directly: where its
// two interfaces (Anthropic Messages and OpenAI-compatible) are on each of
// its hosts, its small model and the models to offer when it cannot be
// asked. Discovery asks the vendor first; this table is only the fallback
// and the way to find the other interface of the same key.
type Vendor struct {
	ID, Name string
	Hosts    []string
	// Paths, when set, are the only base paths on Hosts that are this
	// vendor (opencode.ai serves more than OpenCode Go).
	Paths []string
	// Anthropic and OpenAI are the base paths of its two interfaces.
	Anthropic, OpenAI string
	// Small is the model for Claude Code's background work (its haiku
	// tier); "" runs that work on the chosen model.
	Small  string
	Models []string
}

// Vendors are the built-in catalogue.
var Vendors = []Vendor{
	{ID: "opencode_go", Name: "OpenCode Go", Hosts: []string{"opencode.ai"}, Paths: []string{"/inference/go", "/zen/go"},
		Anthropic: "/inference/go", OpenAI: "/inference/go/v1", Small: "deepseek-v4.1-flash",
		Models: []string{"deepseek-v4.1-flash", "deepseek-v4-pro", "kimi-k3", "glm-5.3", "minimax-m3", "qwen3.8-max", "qwen3.8-flash"}},
	{ID: "deepseek", Name: "DeepSeek", Hosts: []string{"api.deepseek.com"},
		Anthropic: "/anthropic", OpenAI: "/v1", Small: "deepseek-chat",
		Models: []string{"deepseek-chat", "deepseek-reasoner"}},
	{ID: "kimi", Name: "Kimi", Hosts: []string{"api.moonshot.cn", "api.moonshot.ai"},
		Anthropic: "/anthropic", OpenAI: "/v1", Small: "kimi-k2-turbo-preview",
		Models: []string{"kimi-k2-turbo-preview", "kimi-k2-0905-preview"}},
	{ID: "minimax", Name: "MiniMax", Hosts: []string{"api.minimax.io", "api.minimaxi.com"},
		Anthropic: "/anthropic", OpenAI: "/v1",
		Models: []string{"MiniMax-M2"}},
	{ID: "zhipu", Name: "智谱", Hosts: []string{"open.bigmodel.cn", "api.z.ai"},
		Anthropic: "/api/anthropic", OpenAI: "/api/paas/v4", Small: "glm-4.5-air",
		Models: []string{"glm-4.6", "glm-4.5-air"}},
	{ID: "qwen", Name: "通义", Hosts: []string{"dashscope.aliyuncs.com", "dashscope-intl.aliyuncs.com"},
		Anthropic: "/apps/anthropic", OpenAI: "/compatible-mode/v1", Small: "qwen-flash",
		Models: []string{"qwen3-coder-plus", "qwen3-max", "qwen-plus", "qwen-flash"}},
	{ID: "doubao", Name: "豆包", Hosts: []string{"ark.cn-beijing.volces.com"},
		Anthropic: "/api/compatible", OpenAI: "/api/v3",
		Models: []string{"doubao-seed-code-preview-latest"}},
	{ID: "openrouter", Name: "OpenRouter", Hosts: []string{"openrouter.ai"},
		Anthropic: "/api", OpenAI: "/api/v1",
		Models: []string{"deepseek/deepseek-chat", "moonshotai/kimi-k2", "z-ai/glm-4.6", "qwen/qwen3-coder"}},
}

// VendorByID is the catalogue's vendor with this id.
func VendorByID(id string) (Vendor, bool) {
	for _, v := range Vendors {
		if v.ID == id {
			return v, true
		}
	}
	return Vendor{}, false
}

// VendorOf is the catalogue's vendor at base, if any.
func VendorOf(base string) (Vendor, bool) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return Vendor{}, false
	}
	host := strings.ToLower(u.Hostname())
	path := strings.TrimRight(u.Path, "/")
	for _, v := range Vendors {
		for _, h := range v.Hosts {
			if host != h {
				continue
			}
			if len(v.Paths) == 0 {
				return v, true
			}
			for _, p := range v.Paths {
				if path == p || strings.HasPrefix(path, p+"/") {
					return v, true
				}
			}
		}
	}
	return Vendor{}, false
}
