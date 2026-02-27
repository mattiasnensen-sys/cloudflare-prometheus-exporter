package exporter

import (
	"testing"

	"github.com/mattiasnensen-sys/cloudflare-exporter/internal/cloudflare"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestIngestIncrementsCounters(t *testing.T) {
	m, _ := NewMetrics()

	m.Ingest(cloudflare.MetricsSnapshot{
		Zones: []cloudflare.ZoneMetrics{
			{
				ZoneTag: "zone-1",
				RequestSamples: []cloudflare.RequestSample{
					{Hostname: "www.example.com", CacheStatus: "hit", EdgeStatus: 200, Count: 12, EdgeBytes: 5000, Visits: 3},
					{Hostname: "api.example.com", CacheStatus: "miss", EdgeStatus: 503, Count: 2, EdgeBytes: 800, Visits: 1},
				},
				FirewallSamples: []cloudflare.FirewallSample{
					{Action: "block", Source: "waf", Count: 4},
				},
			},
		},
		Workers: []cloudflare.WorkerSample{
			{
				AccountTag:  "acc-1",
				ScriptName:  "voximo-media-gateway",
				Status:      "ok",
				Requests:    100,
				Errors:      2,
				Subrequests: 40,
				CPUTimeP50:  3,
				CPUTimeP99:  11,
			},
		},
	})

	if got := testutil.ToFloat64(m.requestsTotal.WithLabelValues("zone-1", "www.example.com", "hit", "2xx")); got != 12 {
		t.Fatalf("unexpected requestsTotal hit value: %.2f", got)
	}
	if got := testutil.ToFloat64(m.requestsTotal.WithLabelValues("zone-1", "api.example.com", "miss", "5xx")); got != 2 {
		t.Fatalf("unexpected requestsTotal miss value: %.2f", got)
	}
	if got := testutil.ToFloat64(m.edgeBytesTotal.WithLabelValues("zone-1", "www.example.com", "hit")); got != 5000 {
		t.Fatalf("unexpected edgeBytesTotal: %.2f", got)
	}
	if got := testutil.ToFloat64(m.visitsTotal.WithLabelValues("zone-1", "api.example.com")); got != 1 {
		t.Fatalf("unexpected visitsTotal: %.2f", got)
	}
	if got := testutil.ToFloat64(m.firewallEventsTotal.WithLabelValues("zone-1", "block", "waf")); got != 4 {
		t.Fatalf("unexpected firewallEventsTotal: %.2f", got)
	}
	if got := testutil.ToFloat64(m.workersInvocations.WithLabelValues("acc-1", "voximo-media-gateway", "ok")); got != 100 {
		t.Fatalf("unexpected workersInvocations: %.2f", got)
	}
	if got := testutil.ToFloat64(m.workersErrors.WithLabelValues("acc-1", "voximo-media-gateway", "ok")); got != 2 {
		t.Fatalf("unexpected workersErrors: %.2f", got)
	}
	if got := testutil.ToFloat64(m.workersSubrequests.WithLabelValues("acc-1", "voximo-media-gateway", "ok")); got != 40 {
		t.Fatalf("unexpected workersSubrequests: %.2f", got)
	}
	if got := testutil.ToFloat64(m.workersCPUTimeP50.WithLabelValues("acc-1", "voximo-media-gateway", "ok")); got != 3 {
		t.Fatalf("unexpected workersCPUTimeP50: %.2f", got)
	}
	if got := testutil.ToFloat64(m.workersCPUTimeP99.WithLabelValues("acc-1", "voximo-media-gateway", "ok")); got != 11 {
		t.Fatalf("unexpected workersCPUTimeP99: %.2f", got)
	}
}

func TestStatusClassMapping(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{100, "1xx"},
		{200, "2xx"},
		{302, "3xx"},
		{404, "4xx"},
		{503, "5xx"},
		{999, "unknown"},
	}

	for _, tc := range cases {
		if got := toStatusClass(tc.status); got != tc.want {
			t.Fatalf("status %d: got %s, want %s", tc.status, got, tc.want)
		}
	}
}
