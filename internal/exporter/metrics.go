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
	graphqlRequestsTotal *prometheus.CounterVec
	graphqlLastSuccess   prometheus.Gauge
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
				Name: "voximo_cloudflare_http_requests_total",
				Help: "HTTP request counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone", "hostname", "cache_status", "status_class"},
		),
		edgeBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "voximo_cloudflare_http_edge_bytes_total",
				Help: "Edge response bytes from Cloudflare adaptive analytics.",
			},
			[]string{"zone", "hostname", "cache_status"},
		),
		visitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "voximo_cloudflare_http_visits_total",
				Help: "Visit counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone", "hostname"},
		),
		firewallEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "voximo_cloudflare_firewall_events_total",
				Help: "Firewall event counts from Cloudflare adaptive analytics.",
			},
			[]string{"zone", "action", "source"},
		),
		graphqlRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "voximo_cloudflare_api_graphql_requests_total",
				Help: "GraphQL polling attempts grouped by outcome.",
			},
			[]string{"status"},
		),
		graphqlLastSuccess: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "voximo_cloudflare_api_graphql_last_success_timestamp_seconds",
				Help: "Unix timestamp of last successful GraphQL poll.",
			},
		),
		pollDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "voximo_cloudflare_api_poll_duration_seconds",
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
		m.graphqlRequestsTotal,
		m.graphqlLastSuccess,
		m.pollDuration,
	)

	return m, registry
}

// ObserveSuccess records successful poll metrics.
func (m *Metrics) ObserveSuccess(duration time.Duration) {
	m.graphqlRequestsTotal.WithLabelValues("success").Inc()
	m.graphqlLastSuccess.Set(float64(time.Now().Unix()))
	m.pollDuration.Observe(duration.Seconds())

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPollTime = time.Now().UTC()
	m.lastPollError = ""
}

// ObserveError records a failed poll.
func (m *Metrics) ObserveError(duration time.Duration, err error) {
	m.graphqlRequestsTotal.WithLabelValues("error").Inc()
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
func (m *Metrics) Ingest(zoneResults []cloudflare.ZoneMetrics) {
	for _, zone := range zoneResults {
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
