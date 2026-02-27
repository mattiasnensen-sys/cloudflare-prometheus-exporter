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
		if !strings.Contains(query, "CloudflareExporterMetrics") {
			t.Fatalf("expected query to contain CloudflareExporterMetrics")
		}
		if !strings.Contains(query, "accounts(filter: { accountTag: $accountID })") {
			t.Fatalf("expected workers query to include account filter")
		}

		variables, _ := payload["variables"].(map[string]any)
		accountID, _ := variables["accountID"].(string)
		if accountID != "acc-1" {
			t.Fatalf("expected accountID=acc-1, got %+v", variables["accountID"])
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
		                "clientRequestHTTPHost": "www.example.com",
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
		      ],
		      "accounts": [
		        {
		          "accountTag": "acc-1",
		          "workersInvocationsAdaptive": [
		            {
		              "dimensions": {
		                "scriptName": "voximo-media-gateway",
		                "status": "ok"
		              },
		              "sum": {
		                "requests": 100,
		                "errors": 2,
		                "subrequests": 50
		              },
		              "quantiles": {
		                "cpuTimeP50": 3,
		                "cpuTimeP99": 12
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
	res, err := client.FetchMetrics(context.Background(), []string{"zone-1"}, []string{"acc-1"}, RequestWindow{
		MinTime: time.Now().Add(-2 * time.Minute),
		MaxTime: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("FetchMetrics returned error: %v", err)
	}
	if len(res.Zones) != 1 {
		t.Fatalf("expected 1 zone result, got %d", len(res.Zones))
	}

	zone := res.Zones[0]
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
	if req.Hostname != "www.example.com" || req.CacheStatus != "hit" || req.EdgeStatus != 200 {
		t.Fatalf("unexpected request sample: %+v", req)
	}
	if req.Count != 42 || req.EdgeBytes != 12345 || req.Visits != 10 {
		t.Fatalf("unexpected request values: %+v", req)
	}

	fw := zone.FirewallSamples[0]
	if fw.Action != "block" || fw.Source != "waf" || fw.Count != 3 {
		t.Fatalf("unexpected firewall sample: %+v", fw)
	}

	if len(res.Workers) != 1 {
		t.Fatalf("expected 1 worker sample, got %d", len(res.Workers))
	}
	worker := res.Workers[0]
	if worker.AccountTag != "acc-1" || worker.ScriptName != "voximo-media-gateway" || worker.Status != "ok" {
		t.Fatalf("unexpected worker dimensions: %+v", worker)
	}
	if worker.Requests != 100 || worker.Errors != 2 || worker.Subrequests != 50 || worker.CPUTimeP50 != 3 || worker.CPUTimeP99 != 12 {
		t.Fatalf("unexpected worker values: %+v", worker)
	}
}

func TestFetchMetricsGraphQLError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"no access"}]}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token", 10000, 2*time.Second)
	_, err := client.FetchMetrics(context.Background(), []string{"zone-1"}, []string{"acc-1"}, RequestWindow{
		MinTime: time.Now().Add(-2 * time.Minute),
		MaxTime: time.Now().Add(-1 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "graphql error") {
		t.Fatalf("expected graphql error, got %v", err)
	}
}

func TestFetchMetricsWithoutAccountTagsUsesZoneOnlyQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		query, _ := payload["query"].(string)
		if strings.Contains(query, "workersInvocationsAdaptive") {
			t.Fatalf("zone-only query should not request workers metrics")
		}
		variables, _ := payload["variables"].(map[string]any)
		if _, exists := variables["accountID"]; exists {
			t.Fatalf("zone-only query should not set accountID variable")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"viewer": {
					"zones": [
						{
							"zoneTag": "zone-1",
							"httpRequestsAdaptiveGroups": [],
							"firewallEventsAdaptiveGroups": []
						}
					]
				}
			}
		}`))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "token", 10000, 2*time.Second)
	res, err := client.FetchMetrics(context.Background(), []string{"zone-1"}, nil, RequestWindow{
		MinTime: time.Now().Add(-2 * time.Minute),
		MaxTime: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("FetchMetrics returned error: %v", err)
	}
	if len(res.Workers) != 0 {
		t.Fatalf("expected 0 worker samples, got %d", len(res.Workers))
	}
}
