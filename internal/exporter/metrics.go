package exporter

import (
	"sync"
	"time"

	"github.com/mattiasnensen-sys/cloudflare-exporter/internal/cloudflare"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics owns all Prometheus collectors for this exporter.
type Metrics struct {
	mu sync.RWMutex

	requestsTotal        *prometheus.CounterVec
	edgeBytesTotal       *prometheus.CounterVec
	visitsTotal          *prometheus.CounterVec
	firewallEventsTotal  *prometheus.CounterVec
	workersInvocations   *prometheus.CounterVec
	workersErrors        *prometheus.CounterVec
	workersSubrequests   *prometheus.CounterVec
	workersCPUTimeP50    *prometheus.GaugeVec
	workersCPUTimeP99    *prometheus.GaugeVec
	graphqlRequestsTotal *prometheus.CounterVec
	graphqlLastSuccess   *prometheus.GaugeVec
	pollDuration         prometheus.Histogram

	lastPollTime  time.Time
	lastPollError string
}

// NewMetrics initializes and registers collectors in a fresh registry.
func NewMetrics() (*Metrics, *prometheus.Registry) {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.http.requests",
				Help: "HTTP request counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone.tag", "request.hostname", "cache.status", "http.status.class"},
		),
		edgeBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.http.edge.response.bytes",
				Help: "Edge response bytes from Cloudflare adaptive analytics.",
			},
			[]string{"zone.tag", "request.hostname", "cache.status"},
		),
		visitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.http.visits",
				Help: "Visit counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone.tag", "request.hostname"},
		),
		firewallEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.firewall.events",
				Help: "Firewall event counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone.tag", "firewall.action", "firewall.source"},
		),
		workersInvocations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.workers.invocations",
				Help: "Worker invocation counts from Cloudflare workers analytics.",
			},
			[]string{"account.tag", "worker.script", "invocation.status"},
		),
		workersErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.workers.errors",
				Help: "Worker error counts from Cloudflare workers analytics.",
			},
			[]string{"account.tag", "worker.script", "invocation.status"},
		),
		workersSubrequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.workers.subrequests",
				Help: "Worker subrequest counts from Cloudflare workers analytics.",
			},
			[]string{"account.tag", "worker.script", "invocation.status"},
		),
		workersCPUTimeP50: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cloudflare.workers.cpu.time.p50",
				Help: "Worker CPU-time p50 from Cloudflare workers analytics.",
			},
			[]string{"account.tag", "worker.script", "invocation.status"},
		),
		workersCPUTimeP99: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cloudflare.workers.cpu.time.p99",
				Help: "Worker CPU-time p99 from Cloudflare workers analytics.",
			},
			[]string{"account.tag", "worker.script", "invocation.status"},
		),
		graphqlRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cloudflare.api.graphql.requests",
				Help: "GraphQL polling attempts grouped by zone and outcome.",
			},
			[]string{"zone.tag", "poll.status"},
		),
		graphqlLastSuccess: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cloudflare.api.graphql.last_success.timestamp",
				Help: "Unix timestamp of last successful GraphQL poll by zone.",
			},
			[]string{"zone.tag"},
		),
		pollDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "cloudflare.api.graphql.poll.duration",
				Help:    "Duration of Cloudflare GraphQL poll calls.",
				Buckets: prometheus.DefBuckets,
			},
		),
	}

	registry.MustRegister(
		m.requestsTotal,
		m.edgeBytesTotal,
		m.visitsTotal,
		m.firewallEventsTotal,
		m.workersInvocations,
		m.workersErrors,
		m.workersSubrequests,
		m.workersCPUTimeP50,
		m.workersCPUTimeP99,
		m.graphqlRequestsTotal,
		m.graphqlLastSuccess,
		m.pollDuration,
	)

	return m, registry
}

// ObserveSuccess records successful poll metrics.
func (m *Metrics) ObserveSuccess(duration time.Duration, zoneTags []string) {
	now := float64(time.Now().Unix())
	for _, zoneTag := range zoneTags {
		m.graphqlRequestsTotal.WithLabelValues(zoneTag, "success").Inc()
		m.graphqlLastSuccess.WithLabelValues(zoneTag).Set(now)
	}
	m.pollDuration.Observe(duration.Seconds())

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPollTime = time.Now().UTC()
	m.lastPollError = ""
}

// ObserveError records a failed poll.
func (m *Metrics) ObserveError(duration time.Duration, err error) {
	m.graphqlRequestsTotal.WithLabelValues("all", "error").Inc()
	m.pollDuration.Observe(duration.Seconds())

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPollTime = time.Now().UTC()
	if err != nil {
		m.lastPollError = err.Error()
	} else {
		m.lastPollError = "unknown error"
	}
}

// Ingest adds one poll-window result into monotonically increasing counters.
func (m *Metrics) Ingest(result cloudflare.MetricsSnapshot) {
	for _, zone := range result.Zones {
		zoneLabel := zone.ZoneTag
		for _, sample := range zone.RequestSamples {
			statusClass := toStatusClass(sample.EdgeStatus)
			m.requestsTotal.WithLabelValues(zoneLabel, sample.Hostname, sample.CacheStatus, statusClass).Add(sample.Count)
			m.edgeBytesTotal.WithLabelValues(zoneLabel, sample.Hostname, sample.CacheStatus).Add(sample.EdgeBytes)
			m.visitsTotal.WithLabelValues(zoneLabel, sample.Hostname).Add(sample.Visits)
		}
		for _, sample := range zone.FirewallSamples {
			m.firewallEventsTotal.WithLabelValues(zoneLabel, sample.Action, sample.Source).Add(sample.Count)
		}
	}

	for _, sample := range result.Workers {
		labels := []string{sample.AccountTag, sample.ScriptName, sample.Status}
		m.workersInvocations.WithLabelValues(labels...).Add(sample.Requests)
		m.workersErrors.WithLabelValues(labels...).Add(sample.Errors)
		m.workersSubrequests.WithLabelValues(labels...).Add(sample.Subrequests)
		m.workersCPUTimeP50.WithLabelValues(labels...).Set(sample.CPUTimeP50)
		m.workersCPUTimeP99.WithLabelValues(labels...).Set(sample.CPUTimeP99)
	}
}

// HealthState exposes latest poll status for readiness endpoints.
type HealthState struct {
	LastPollTime  time.Time `json:"last_poll_time"`
	LastPollError string    `json:"last_poll_error,omitempty"`
}

// Health returns latest in-memory poll status.
func (m *Metrics) Health() HealthState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return HealthState{LastPollTime: m.lastPollTime, LastPollError: m.lastPollError}
}

func toStatusClass(status int) string {
	if status < 200 {
		return "1xx"
	}
	if status < 300 {
		return "2xx"
	}
	if status < 400 {
		return "3xx"
	}
	if status < 500 {
		return "4xx"
	}
	if status < 600 {
		return "5xx"
	}
	return "unknown"
}
