package perf

import (
	"context"
	"testing"
	"time"
)

func TestRequestStartSurvivesRouteAndWaitAnnotations(t *testing.T) {
	if !RequestStart(context.Background()).IsZero() {
		t.Fatal("uninstrumented request acquired an invented ingress time")
	}
	start := time.Unix(1700000000, 123000000)
	ctx := WithRoute(context.Background(), start)
	SetRoute(ctx, "POST /v1/messages")
	MarkWaiting(ctx)
	if RequestStart(ctx) != start || Route(ctx) != "POST /v1/messages" || !Waiting(ctx) {
		t.Fatal("later annotations changed the outer request's start time")
	}
}
