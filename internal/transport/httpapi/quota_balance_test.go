package httpapi

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func TestQuotaBalanceRoundTrip(t *testing.T) {
	n, left := 50.0, 17.5
	q := &api.Quota{Provider: "openrouter", Source: "openrouter", Status: "ok", At: time.Now(), CheckedAt: time.Now(), Windows: []api.QuotaWindow{{Name: "balance", Used: .65, Balance: &api.QuotaBalance{Currency: "USD", Limit: &n, Remaining: &left}}}}
	got := quotaDTO(quotaFromAPI(q))
	if !reflect.DeepEqual(q, got) {
		a, _ := json.Marshal(q)
		b, _ := json.Marshal(got)
		t.Fatalf("quota balance round-trip changed: %s vs %s", a, b)
	}
}
