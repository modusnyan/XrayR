package mydispatcher

import (
	"context"
	"testing"

	appstats "github.com/xtls/xray-core/app/stats"
	featstats "github.com/xtls/xray-core/features/stats"
)

// benchEmail is a package-level variable rather than a constant so the compiler
// cannot constant-fold the counter names. A real user email is a runtime value,
// and folding would hide the allocation the concatenation actually costs.
var benchEmail = "user@example.com"

// BenchmarkUserTrafficCounterLookup measures what getLink does per connection to
// wire up the per-user traffic counters: build two names by concatenation and
// look each one up in the stats manager.
//
// The counters are registered once and then only looked up, which is the steady
// state for a node that is already serving users.
func BenchmarkUserTrafficCounterLookup(b *testing.B) {
	manager, err := appstats.NewManager(context.Background(), &appstats.Config{})
	if err != nil {
		b.Fatal(err)
	}
	if _, err := featstats.GetOrRegisterCounter(manager, "user>>>"+benchEmail+">>>traffic>>>uplink"); err != nil {
		b.Fatal(err)
	}
	if _, err := featstats.GetOrRegisterCounter(manager, "user>>>"+benchEmail+">>>traffic>>>downlink"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		up := "user>>>" + benchEmail + ">>>traffic>>>uplink"
		down := "user>>>" + benchEmail + ">>>traffic>>>downlink"
		if _, err := featstats.GetOrRegisterCounter(manager, up); err != nil {
			b.Fatal(err)
		}
		if _, err := featstats.GetOrRegisterCounter(manager, down); err != nil {
			b.Fatal(err)
		}
	}
}
