package rule

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
)

// blockRules builds count distinct block rules, which is what a panel with an
// audit-rule list pushes down.
func blockRules(count int) []api.DetectRule {
	rules := make([]api.DetectRule, 0, count)
	for i := 0; i < count; i++ {
		rules = append(rules, api.DetectRule{
			ID:      i,
			Pattern: regexp.MustCompile(fmt.Sprintf(`(?i)^blocked-%d\.example\.com$`, i)),
		})
	}
	return rules
}

func newManagerWithRules(t testing.TB, count int) *Manager {
	t.Helper()
	m := New()
	require.NoError(t, m.UpdateRule("tag", blockRules(count)))
	return m
}

func TestDetectRejectsAMatchingDestination(t *testing.T) {
	m := newManagerWithRules(t, 3)
	assert.True(t, m.Detect("tag", "blocked-1.example.com", "tag|user@example.com|42"))
}

func TestDetectAllowsANonMatchingDestination(t *testing.T) {
	m := newManagerWithRules(t, 3)
	assert.False(t, m.Detect("tag", "allowed.example.com", "tag|user@example.com|42"))
}

func TestDetectIsANoOpWithoutRules(t *testing.T) {
	assert.False(t, New().Detect("tag", "anything.example.com", "tag|user@example.com|42"))
}

func TestDetectRecordsTheRuleThatMatched(t *testing.T) {
	m := newManagerWithRules(t, 3)
	require.True(t, m.Detect("tag", "blocked-2.example.com", "tag|user@example.com|42"))

	results, err := m.GetDetectResult("tag")
	require.NoError(t, err)
	require.Len(t, *results, 1)
	assert.Equal(t, 42, (*results)[0].UID)
	assert.Equal(t, 2, (*results)[0].RuleID)
}

// BenchmarkDetectMiss measures the common case: a destination that no rule
// matches, so every rule is evaluated. This runs once per connection.
func BenchmarkDetectMiss(b *testing.B) {
	for _, count := range []int{10, 100, 1000} {
		m := newManagerWithRules(b, count)
		b.Run(fmt.Sprintf("rules=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if m.Detect("tag", "allowed.example.com", "tag|user@example.com|42") {
					b.Fatal("unexpected reject")
				}
			}
		})
	}
}

// BenchmarkDetectHit measures the case where the last rule matches, so the whole
// list is scanned before the reject is recorded.
func BenchmarkDetectHit(b *testing.B) {
	m := newManagerWithRules(b, 100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !m.Detect("tag", "blocked-99.example.com", "tag|user@example.com|42") {
			b.Fatal("expected reject")
		}
	}
}
