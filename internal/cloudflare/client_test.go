package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchMetricsSuccess(t *testing.T) {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected bearer token, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}

		query, _ := payload["query"].(string)
		if !strings.Contains(query, "VoximoMetrics") {
			t.Fatalf("expected query to contain VoximoMetrics")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`
		{
		  "data": {
		    "viewer": {
		      "zones": [
		        {
		          "zoneTag": "zone-1",
		          "httpRequestsAdaptiveGroups": [
		            {
		              "count": 42,
		              "dimensions": {
		                "clientRequestHTTPHost": "www.voximo.eu",
		                "cacheStatus": "hit",
		                "edgeResponseStatus": 200
		              },
		              "sum": {
		                "edgeResponseBytes": 12345,
		                "visits": 10
		              }
		            }
		          ],
		          "firewallEventsAdaptiveGroups": [
		            {
		              "count": 3,
		              "dimensions": {
		                "action": "block",
		                "source": "waf"
		              }
		            }
		          ]
		        }
		      ]
		    }
		  }
		}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token-123", 10000, 2*time.Second)
	res, err := client.FetchMetrics(context.Background(), []string{"zone-1"}, RequestWindow{
		MinTime: time.Now().Add(-2 * time.Minute),
		MaxTime: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("FetchMetrics returned error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 zone result, got %d", len(res))
	}

	zone := res[0]
	if zone.ZoneTag != "zone-1" {
		t.Fatalf("unexpected zone tag: %s", zone.ZoneTag)
	}
	if len(zone.RequestSamples) != 1 {
		t.Fatalf("expected 1 request sample, got %d", len(zone.RequestSamples))
	}
	if len(zone.FirewallSamples) != 1 {
		t.Fatalf("expected 1 firewall sample, got %d", len(zone.FirewallSamples))
	}

	req := zone.RequestSamples[0]
	if req.Hostname != "www.voximo.eu" || req.CacheStatus != "hit" || req.EdgeStatus != 200 {
		t.Fatalf("unexpected request sample: %+v", req)
	}
	if req.Count != 42 || req.EdgeBytes != 12345 || req.Visits != 10 {
		t.Fatalf("unexpected request values: %+v", req)
	}

	fw := zone.FirewallSamples[0]
	if fw.Action != "block" || fw.Source != "waf" || fw.Count != 3 {
		t.Fatalf("unexpected firewall sample: %+v", fw)
	}
}

func TestFetchMetricsGraphQLError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"no access"}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token", 10000, 2*time.Second)
	_, err := client.FetchMetrics(context.Background(), []string{"zone-1"}, RequestWindow{
		MinTime: time.Now().Add(-2 * time.Minute),
		MaxTime: time.Now().Add(-1 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "graphql error") {
		t.Fatalf("expected graphql error, got %v", err)
	}
}
