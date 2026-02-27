package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const zoneMetricsQuery = `
query CloudflareExporterZoneMetrics($zoneIDs: [string!], $mintime: Time!, $maxtime: Time!, $limit: uint64!) {
  viewer {
    zones(filter: { zoneTag_in: $zoneIDs }) {
      zoneTag
      httpRequestsAdaptiveGroups(
        limit: $limit
        filter: { datetime_geq: $mintime, datetime_lt: $maxtime }
      ) {
        count
        dimensions {
          clientRequestHTTPHost
          cacheStatus
          edgeResponseStatus
        }
        sum {
          edgeResponseBytes
          visits
        }
      }
      firewallEventsAdaptiveGroups(
        limit: $limit
        filter: { datetime_geq: $mintime, datetime_lt: $maxtime }
      ) {
        count
        dimensions {
          action
          source
        }
      }
    }
  }
}
`

const zoneAndWorkerMetricsQuery = `
query CloudflareExporterMetrics($zoneIDs: [string!], $accountIDs: [string!], $mintime: Time!, $maxtime: Time!, $limit: uint64!) {
  viewer {
    zones(filter: { zoneTag_in: $zoneIDs }) {
      zoneTag
      httpRequestsAdaptiveGroups(
        limit: $limit
        filter: { datetime_geq: $mintime, datetime_lt: $maxtime }
      ) {
        count
        dimensions {
          clientRequestHTTPHost
          cacheStatus
          edgeResponseStatus
        }
        sum {
          edgeResponseBytes
          visits
        }
      }
      firewallEventsAdaptiveGroups(
        limit: $limit
        filter: { datetime_geq: $mintime, datetime_lt: $maxtime }
      ) {
        count
        dimensions {
          action
          source
        }
      }
    }
    accounts(filter: { accountTag_in: $accountIDs }) {
      accountTag
      workersInvocationsAdaptive(
        limit: $limit
        filter: { datetime_geq: $mintime, datetime_lt: $maxtime }
      ) {
        dimensions {
          scriptName
          status
        }
        sum {
          requests
          errors
          subrequests
        }
        quantiles {
          cpuTimeP50
          cpuTimeP99
        }
      }
    }
  }
}
`

// Client fetches Cloudflare analytics data via GraphQL.
type Client struct {
	endpoint   string
	apiToken   string
	queryLimit int
	httpClient *http.Client
}

// NewClient creates a Cloudflare GraphQL client.
func NewClient(endpoint, apiToken string, queryLimit int, timeout time.Duration) *Client {
	return &Client{
		endpoint:   endpoint,
		apiToken:   apiToken,
		queryLimit: queryLimit,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// RequestWindow is the query time range.
type RequestWindow struct {
	MinTime time.Time
	MaxTime time.Time
}

// ZoneMetrics contains parsed metrics for one zone.
type ZoneMetrics struct {
	ZoneTag         string
	RequestSamples  []RequestSample
	FirewallSamples []FirewallSample
}

// WorkerSample captures Workers runtime dimensions and aggregates.
type WorkerSample struct {
	AccountTag  string
	ScriptName  string
	Status      string
	Requests    float64
	Errors      float64
	Subrequests float64
	CPUTimeP50  float64
	CPUTimeP99  float64
}

// MetricsSnapshot contains one poll-window result.
type MetricsSnapshot struct {
	Zones   []ZoneMetrics
	Workers []WorkerSample
}

// RequestSample captures request-level dimensions.
type RequestSample struct {
	Hostname    string
	CacheStatus string
	EdgeStatus  int
	Count       float64
	EdgeBytes   float64
	Visits      float64
}

// FirewallSample captures firewall-level dimensions.
type FirewallSample struct {
	Action string
	Source string
	Count  float64
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data struct {
		Viewer struct {
			Zones []struct {
				ZoneTag                    string `json:"zoneTag"`
				HTTPRequestsAdaptiveGroups []struct {
					Count      float64 `json:"count"`
					Dimensions struct {
						ClientRequestHTTPHost string `json:"clientRequestHTTPHost"`
						CacheStatus           string `json:"cacheStatus"`
						EdgeResponseStatus    int    `json:"edgeResponseStatus"`
					} `json:"dimensions"`
					Sum struct {
						EdgeResponseBytes float64 `json:"edgeResponseBytes"`
						Visits            float64 `json:"visits"`
					} `json:"sum"`
				} `json:"httpRequestsAdaptiveGroups"`
				FirewallEventsAdaptiveGroups []struct {
					Count      float64 `json:"count"`
					Dimensions struct {
						Action string `json:"action"`
						Source string `json:"source"`
					} `json:"dimensions"`
				} `json:"firewallEventsAdaptiveGroups"`
			} `json:"zones"`
			Accounts []struct {
				AccountTag                 string `json:"accountTag"`
				WorkersInvocationsAdaptive []struct {
					Dimensions struct {
						ScriptName string `json:"scriptName"`
						Status     string `json:"status"`
					} `json:"dimensions"`
					Sum struct {
						Requests    float64 `json:"requests"`
						Errors      float64 `json:"errors"`
						Subrequests float64 `json:"subrequests"`
					} `json:"sum"`
					Quantiles struct {
						CPUTimeP50 float64 `json:"cpuTimeP50"`
						CPUTimeP99 float64 `json:"cpuTimeP99"`
					} `json:"quantiles"`
				} `json:"workersInvocationsAdaptive"`
			} `json:"accounts"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

// FetchMetrics fetches one analytics window for the provided zones and accounts.
func (c *Client) FetchMetrics(ctx context.Context, zoneTags []string, accountTags []string, window RequestWindow) (MetricsSnapshot, error) {
	vars := map[string]interface{}{
		"zoneIDs": zoneTags,
		"mintime": window.MinTime.UTC().Format(time.RFC3339),
		"maxtime": window.MaxTime.UTC().Format(time.RFC3339),
		"limit":   c.queryLimit,
	}
	query := zoneMetricsQuery
	if len(accountTags) > 0 {
		vars["accountIDs"] = accountTags
		query = zoneAndWorkerMetricsQuery
	}

	payload, err := json.Marshal(graphQLRequest{Query: query, Variables: vars})
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("build graphql request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("execute graphql request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return MetricsSnapshot{}, fmt.Errorf("graphql status %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("decode graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return MetricsSnapshot{}, fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	out := MetricsSnapshot{
		Zones:   make([]ZoneMetrics, 0, len(gqlResp.Data.Viewer.Zones)),
		Workers: make([]WorkerSample, 0),
	}

	for _, z := range gqlResp.Data.Viewer.Zones {
		m := ZoneMetrics{ZoneTag: z.ZoneTag}
		for _, r := range z.HTTPRequestsAdaptiveGroups {
			m.RequestSamples = append(m.RequestSamples, RequestSample{
				Hostname:    safeLabel(r.Dimensions.ClientRequestHTTPHost, "unknown"),
				CacheStatus: safeLabel(r.Dimensions.CacheStatus, "unknown"),
				EdgeStatus:  r.Dimensions.EdgeResponseStatus,
				Count:       nonNegative(r.Count),
				EdgeBytes:   nonNegative(r.Sum.EdgeResponseBytes),
				Visits:      nonNegative(r.Sum.Visits),
			})
		}
		for _, f := range z.FirewallEventsAdaptiveGroups {
			m.FirewallSamples = append(m.FirewallSamples, FirewallSample{
				Action: safeLabel(f.Dimensions.Action, "unknown"),
				Source: safeLabel(f.Dimensions.Source, "unknown"),
				Count:  nonNegative(f.Count),
			})
		}
		out.Zones = append(out.Zones, m)
	}

	for _, account := range gqlResp.Data.Viewer.Accounts {
		accountTag := safeLabel(account.AccountTag, "unknown")
		for _, w := range account.WorkersInvocationsAdaptive {
			out.Workers = append(out.Workers, WorkerSample{
				AccountTag:  accountTag,
				ScriptName:  safeLabel(w.Dimensions.ScriptName, "unknown"),
				Status:      safeLabel(w.Dimensions.Status, "unknown"),
				Requests:    nonNegative(w.Sum.Requests),
				Errors:      nonNegative(w.Sum.Errors),
				Subrequests: nonNegative(w.Sum.Subrequests),
				CPUTimeP50:  nonNegative(w.Quantiles.CPUTimeP50),
				CPUTimeP99:  nonNegative(w.Quantiles.CPUTimeP99),
			})
		}
	}

	return out, nil
}

func safeLabel(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func nonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
