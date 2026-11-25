package controller

import (
	"testing"
	"time"
)

// These benchmarks measure the work the control-plane additions perform. The
// data plane (app/mydispatcher, common/limiter) is untouched: its machine code is
// identical to the previous release, so only these paths add cost.
//
// Read them per tick, not per second: each runs once per UpdatePeriodic, which
// defaults to 60 seconds (the certificate monitor runs once per UpdatePeriodic
// minutes).

// BenchmarkCoreHeartbeatProbe covers the per-tick cost of the liveness probe:
// one loopback TCP connection to the node's own listener.
func BenchmarkCoreHeartbeatProbe(b *testing.B) {
	silenceLogs(b)
	c := heartbeatController(b, listeningPort(b))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.coreHeartbeatMonitor(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCertificateDaysRemaining covers the per-tick cost of reading and
// parsing the leaf certificate. It runs once per UpdatePeriodic minutes.
func BenchmarkCertificateDaysRemaining(b *testing.B) {
	path := writeCertificate(b, 48*time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := certificateDaysRemaining(path); err != nil {
			b.Fatal(err)
		}
	}
}
