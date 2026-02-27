# cloudflare-exporter

Standalone Prometheus exporter for Cloudflare edge analytics.

This project polls Cloudflare GraphQL Analytics API and exposes low-cardinality metrics for scraping by Prometheus or OpenTelemetry Collector.

## Why this project

- Pure standalone service (no Worker runtime dependency)
- Designed for OTel Collector -> ClickHouse pipelines
- Low-cardinality metric model suitable for long-term storage

## Exported Metrics

- `voximo_cloudflare_http_requests_total{zone,hostname,cache_status,status_class}`
- `voximo_cloudflare_http_edge_bytes_total{zone,hostname,cache_status}`
- `voximo_cloudflare_http_visits_total{zone,hostname}`
- `voximo_cloudflare_firewall_events_total{zone,action,source}`
- `voximo_cloudflare_api_graphql_requests_total{status}`
- `voximo_cloudflare_api_graphql_last_success_timestamp_seconds`
- `voximo_cloudflare_api_poll_duration_seconds`

## Configuration

All configuration is provided via environment variables.

- `CLOUDFLARE_API_TOKEN` (required): API token with analytics read scope
- `CLOUDFLARE_ZONE_TAGS` (required): comma-separated zone IDs
- `CLOUDFLARE_GRAPHQL_ENDPOINT` (optional): default `https://api.cloudflare.com/client/v4/graphql`
- `POLL_INTERVAL` (optional): default `60s`
- `WINDOW_DURATION` (optional): default `60s`
- `SCRAPE_DELAY` (optional): default `120s`
- `QUERY_LIMIT` (optional): default `10000`
- `REQUEST_TIMEOUT` (optional): default `20s`
- `PORT` (optional): default `9103`
- `METRICS_PATH` (optional): default `/metrics`
- `HEALTH_PATH` (optional): default `/healthz`

## Cloudflare Token Permissions

Minimum expected permissions:

- Zone -> Analytics: Read
- Zone -> Zone: Read

If your tenant/API route requires account-level analytics access, also add:

- Account -> Account Analytics: Read

## Local Run

```bash
export CLOUDFLARE_API_TOKEN="<token>"
export CLOUDFLARE_ZONE_TAGS="<zone-id-1>,<zone-id-2>"
go run ./cmd/cloudflare-exporter
```

Then open:

- `http://localhost:9103/metrics`
- `http://localhost:9103/healthz`

## Prometheus Scrape Example

```yaml
scrape_configs:
  - job_name: cloudflare
    scrape_interval: 60s
    static_configs:
      - targets: ["cloudflare-exporter:9103"]
```

## OpenTelemetry Collector Scrape Example

```yaml
receivers:
  prometheus/cloudflare:
    config:
      scrape_configs:
        - job_name: cloudflare
          scrape_interval: 60s
          static_configs:
            - targets: ["cloudflare-exporter:9103"]
          metric_relabel_configs:
            - source_labels: [__name__]
              regex: "voximo_cloudflare_.*"
              action: keep
```

## Notes

- This exporter adds each poll window to monotonic counters.
- Counters reset on process restart, which is normal Prometheus behavior.
- Keep hostname cardinality bounded at Cloudflare side (host filtering) if needed.
