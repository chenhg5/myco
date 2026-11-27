package feishu

// Creating a bot by scanning a QR code: Feishu's app registration device
// flow (accounts.feishu.cn /oauth/v1/app/registration). The person scans
// with the Feishu or Lark app; Feishu creates a personal bot with its
// permissions and long-connection events set up, and hands back its
// credentials and the person's open_id. Ported from chenhg5/cc-connect
// (MIT), cmd/cc-connect/feishu.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	accountsFeishu = "https://accounts.feishu.cn"
	accountsLark   = "https://accounts.larksuite.com"
)

// Registration is what a finished scan gives.
type Registration struct {
	AppID, AppSecret string
	OwnerOpenID      string
	Platform         string // feishu or lark (from the scanner's tenant)
}

// Registrar runs the flow; BaseURL and HTTP are for tests.
type Registrar struct {
	BaseURL string
	HTTP    *http.Client
	// LarkURL replaces accountsLark (tests).
	LarkURL string
	// Lark: register with Lark (the international app and its accounts),
	// not Feishu. The two are separate: a Feishu code does not open in Lark.
	Lark bool
}

// Pending is a registration waiting to be scanned: show URL as a QR code.
type Pending struct {
	URL        string
	deviceCode string
	interval   time.Duration
	expires    time.Time
	r          *Registrar
	base       string
}

type regResponse struct {
	Error            string   `json:"error"`
	ErrorDescription string   `json:"error_description"`
	SupportedAuth    []string `json:"supported_auth_methods"`
	DeviceCode       string   `json:"device_code"`
	VerificationURI  string   `json:"verification_uri_complete"`
	Interval         int      `json:"interval"`
	ExpiresIn        int      `json:"expires_in"`
	ExpireIn         int      `json:"expire_in"` // older answers
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret"`
	UserInfo         struct {
		OpenID      string `json:"open_id"`
		TenantBrand string `json:"tenant_brand"`
	} `json:"user_info"`
}

func (r *Registrar) base() string {
	if r.Lark {
		return r.larkURL()
	}
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return accountsFeishu
}

func (r *Registrar) larkURL() string {
	if r.LarkURL != "" {
		return r.LarkURL
	}
	return accountsLark
}

func (r *Registrar) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (r *Registrar) call(ctx context.Context, base, action string, params map[string]string) (*regResponse, error) {
	form := url.Values{"action": {action}}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out regResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("registration %s: HTTP %d, unreadable answer", action, resp.StatusCode)
	}
	return &out, nil
}

// Begin asks Feishu for a registration to scan.
func (r *Registrar) Begin(ctx context.Context) (*Pending, error) {
	base := r.base()
	init, err := r.call(ctx, base, "init", nil)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if init.Error != "" {
		return nil, fmt.Errorf("init: %s: %s", init.Error, init.ErrorDescription)
	}
	if len(init.SupportedAuth) > 0 && !contains(init.SupportedAuth, "client_secret") {
		return nil, errors.New("this Feishu environment does not offer client_secret apps")
	}
	b, err := r.call(ctx, base, "begin", map[string]string{
		"archetype": "PersonalAgent", "auth_method": "client_secret", "request_user_info": "open_id"})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	if b.Error != "" {
		return nil, fmt.Errorf("begin: %s: %s", b.Error, b.ErrorDescription)
	}
	if b.DeviceCode == "" || b.VerificationURI == "" {
		return nil, errors.New("begin: incomplete answer from Feishu")
	}
	p := &Pending{URL: b.VerificationURI, deviceCode: b.DeviceCode, r: r, base: base, interval: 5 * time.Second}
	if b.Interval > 0 {
		p.interval = time.Duration(b.Interval) * time.Second
	}
	if exp := max(b.ExpiresIn, b.ExpireIn); exp > 0 {
		p.expires = time.Now().Add(time.Duration(exp) * time.Second)
	}
	return p, nil
}

// Wait polls until the person has scanned and approved, or ctx ends.
func (p *Pending) Wait(ctx context.Context) (*Registration, error) {
	platform := "feishu"
	if p.r.Lark {
		platform = "lark"
	}
	lark := p.r.larkURL()
	for {
		if !p.expires.IsZero() && time.Now().After(p.expires) {
			return nil, errors.New("the QR code expired; run the command again")
		}
		res, err := p.r.call(ctx, p.base, "poll", map[string]string{"device_code": p.deviceCode})
		if err != nil {
			return nil, fmt.Errorf("poll: %w", err)
		}
		if strings.EqualFold(strings.TrimSpace(res.UserInfo.TenantBrand), "lark") {
			platform = "lark"
			if p.base != lark {
				p.base = lark // Lark tenants finish on the Lark domain
				continue
			}
		}
		if res.ClientID != "" && res.ClientSecret != "" {
			return &Registration{AppID: res.ClientID, AppSecret: res.ClientSecret,
				OwnerOpenID: res.UserInfo.OpenID, Platform: platform}, nil
		}
		switch res.Error {
		case "", "authorization_pending":
		case "slow_down":
			p.interval += 5 * time.Second
		case "access_denied":
			return nil, errors.New("the registration was declined in the Feishu app")
		case "expired_token":
			return nil, errors.New("the QR code expired; run the command again")
		default:
			return nil, fmt.Errorf("%s: %s", res.Error, res.ErrorDescription)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.interval):
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return true
		}
	}
	return false
}
