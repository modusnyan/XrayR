package observability

import "github.com/prometheus/client_golang/prometheus"

var (
	PanelRequests     = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "xrayr_panel_requests_total", Help: "Panel API requests by operation and result."}, []string{"panel", "operation", "result"})
	LastSync          = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "xrayr_last_sync_timestamp_seconds", Help: "Unix timestamp of the last successful node synchronization."}, []string{"panel", "node_type"})
	Users             = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "xrayr_users", Help: "Current configured users."}, []string{"panel", "node_type"})
	TrafficFailures   = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "xrayr_traffic_report_failures_total", Help: "Failed traffic reports."}, []string{"panel"})
	Reloads           = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "xrayr_config_reloads_total", Help: "Configuration reload attempts."}, []string{"result"})
	CertDaysRemaining = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "xrayr_certificate_days_remaining", Help: "Certificate validity remaining in days."}, []string{"node_type"})
)

func init() {
	prometheus.MustRegister(PanelRequests, LastSync, Users, TrafficFailures, Reloads, CertDaysRemaining)
}
