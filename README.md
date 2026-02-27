# cloudflare-exporter

Standalone Prometheus exporter for Cloudflare edge analytics.

This project polls Cloudflare GraphQL Analytics API and exposes low-cardinality metrics for scraping by Prometheus or OpenTelemetry Collector.

## Why this project

- Pure standalone service (no Worker runtime dependency)
- Designed for OTel Collector -> ClickHouse pipelines
- Low-cardinality metric model suitable for long-term storage

## Exported Metrics

- `cloudflare.http.requests{zone.tag,request.hostname,cache.status,http.status.class}`
- `cloudflare.http.edge.response.bytes{zone.tag,request.hostname,cache.status}`
- `cloudflare.http.visits{zone.tag,request.hostname}`
- `cloudflare.firewall.events{zone.tag,firewall.action,firewall.source}`
- `cloudflare.workers.invocations{account.tag,worker.script,invocation.status}`
- `cloudflare.workers.errors{account.tag,worker.script,invocation.status}`
- `cloudflare.workers.subrequests{account.tag,worker.script,invocation.status}`
- `cloudflare.workers.cpu.time.p50{account.tag,worker.script,invocation.status}`
- `cloudflare.workers.cpu.time.p99{account.tag,worker.script,invocation.status}`
- `cloudflare.api.graphql.requests{zone.tag,poll.status}`
- `cloudflare.api.graphql.last_success.timestamp{zone.tag}`
- `cloudflare.api.graphql.poll.duration`

## Configuration

All configuration is provided via environment variables.

- `CLOUDFLARE_API_TOKEN` (required): API token with analytics read scope
- `CLOUDFLARE_ZONE_TAGS` (required): comma-separated zone IDs
- `CLOUDFLARE_ACCOUNT_TAGS` (optional): comma-separated account IDs used for Workers analytics
- `CLOUDFLARE_ACCOUNT_ID` (optional): single account ID fallback if `CLOUDFLARE_ACCOUNT_TAGS` is unset
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
- Account -> Account Analytics: Read (required if Workers metrics are enabled via account IDs)

## Local Run

```bash
export CLOUDFLARE_API_TOKEN="<token>"
export CLOUDFLARE_ZONE_TAGS="<zone-id-1>,<zone-id-2>"
export CLOUDFLARE_ACCOUNT_ID="<account-id>"
go run ./cmd/cloudflare-exporter
```

Then open:

- `http://localhost:9103/metrics`
- `http://localhost:9103/healthz`

## Helm Chart

A deployable chart is included in this repository:

- `charts/cloudflare-exporter`

Install example:

```bash
helm upgrade --install cloudflare-exporter ./charts/cloudflare-exporter \
  --namespace otel --create-namespace \
  --set cloudflare.existingSecret.name=cloudflare-exporter-credentials
```

If you want Helm to create the credentials secret directly:

```bash
helm upgrade --install cloudflare-exporter ./charts/cloudflare-exporter \
  --namespace otel --create-namespace \
  --set secret.create=true \
  --set secret.apiToken="<token>" \
  --set secret.zoneTags="<zone-id-1>,<zone-id-2>"
```

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
              regex: "cloudflare\\..*"
              action: keep
```

## Notes

- This exporter adds each poll window to monotonic counters.
- Counters reset on process restart, which is normal Prometheus behavior.
- Keep hostname cardinality bounded at Cloudflare side (host filtering) if needed.

## CI/CD

GitHub Actions workflows included:

- `.github/workflows/ci.yaml`:
  - `go mod tidy` + `gofmt` check (via `make tidy` + `make fmt`)
  - `git diff --exit-code` guard for generated/format changes
  - `go vet`
  - `go test -race -covermode=atomic -coverprofile=coverage.out ./...`
  - upload coverage artifact
  - Docker build smoke test
  - `helm lint` + `helm template`
- `.github/workflows/image.yaml`:
  - publish snapshot image to `ghcr.io/<owner>/cloudflare-exporter` on `main`
- `.github/workflows/release-image.yaml`:
  - verify release inputs and publish semver-tagged multi-arch image on `v*.*.*`
- `.github/workflows/helm-publish.yaml`:
  - auto-update `charts/cloudflare-exporter/Chart.yaml` `version` + `appVersion` from `chart-v*.*.*` tag
  - commit updated `Chart.yaml` to `main`
  - package and push chart to OCI registry `ghcr.io/<owner>/charts/cloudflare-exporter`
