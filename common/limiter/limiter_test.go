package limiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
)

func testLimiter(t testing.TB, users *[]api.UserInfo) *Limiter {
	t.Helper()
	l := New()
	require.NoError(t, l.AddInboundLimiter("tag", 0, users, nil))
	return l
}

func TestGetUserBucketRejectsBeyondTheDeviceLimit(t *testing.T) {
	users := []api.UserInfo{{UID: 1, Email: "a@example.com", DeviceLimit: 1}}
	l := testLimiter(t, &users)

	_, _, reject := l.GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1")
	assert.False(t, reject, "first device")

	_, _, reject = l.GetUserBucket("tag", "tag|a@example.com|1", "2.2.2.2")
	assert.True(t, reject, "second device exceeds a limit of one")
}

func TestGetUserBucketReturnsABucketOnlyWhenLimited(t *testing.T) {
	unlimited := []api.UserInfo{{UID: 1, Email: "a@example.com"}}
	_, limited, _ := testLimiter(t, &unlimited).GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1")
	assert.False(t, limited)

	throttled := []api.UserInfo{{UID: 1, Email: "a@example.com", SpeedLimit: 1000}}
	bucket, limited, _ := testLimiter(t, &throttled).GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1")
	assert.True(t, limited)
	require.NotNil(t, bucket)
}

func TestGetUserBucketReusesTheSameBucket(t *testing.T) {
	users := []api.UserInfo{{UID: 1, Email: "a@example.com", SpeedLimit: 1000}}
	l := testLimiter(t, &users)

	first, _, _ := l.GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1")
	second, _, _ := l.GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1")
	assert.Same(t, first, second, "a per-user bucket must be shared across connections")
}

func TestGetUserBucketReportsAnUnknownInbound(t *testing.T) {
	bucket, limited, reject := New().GetUserBucket("missing", "tag|a@example.com|1", "1.1.1.1")
	assert.Nil(t, bucket)
	assert.False(t, limited)
	assert.False(t, reject)
}

// BenchmarkGetUserBucket measures the per-connection limiter lookup. This runs
// once for every connection that carries a user, so its allocations are paid on
// the connection path.
func BenchmarkGetUserBucket(b *testing.B) {
	for name, users := range map[string][]api.UserInfo{
		"unlimited":   {{UID: 1, Email: "a@example.com"}},
		"throttled":   {{UID: 1, Email: "a@example.com", SpeedLimit: 1000}},
		"devicelimit": {{UID: 1, Email: "a@example.com", DeviceLimit: 3}},
	} {
		l := testLimiter(b, &users)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, reject := l.GetUserBucket("tag", "tag|a@example.com|1", "1.1.1.1"); reject {
					b.Fatal("unexpected reject")
				}
			}
		})
	}
}
