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

const metricsQuery = `
query VoximoMetrics($zoneIDs: [string!], $mintime: Time!, $maxtime: Time!, $limit: uint64!) {
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
		} `json:"viewer"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

// FetchMetrics fetches one analytics window for the provided zones.
func (c *Client) FetchMetrics(ctx context.Context, zoneTags []string, window RequestWindow) ([]ZoneMetrics, error) {
	vars := map[string]interface{}{
		"zoneIDs": zoneTags,
		"mintime": window.MinTime.UTC().Format(time.RFC3339),
		"maxtime": window.MaxTime.UTC().Format(time.RFC3339),
		"limit":   c.queryLimit,
	}

	payload, err := json.Marshal(graphQLRequest{Query: metricsQuery, Variables: vars})
	if err != nil {
		return nil, fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build graphql request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute graphql request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql status %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	out := make([]ZoneMetrics, 0, len(gqlResp.Data.Viewer.Zones))
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
		out = append(out, m)
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
