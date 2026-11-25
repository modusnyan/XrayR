package api

import (
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
)

// BenchmarkObservePanelRequest measures the cost of one counter increment, which
// is the only work the panel metrics add. It runs once per panel API request,
// i.e. a handful of times per UpdatePeriodic.
func BenchmarkObservePanelRequest(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ObservePanelRequest("Xboard", "/api/v1/server/UniProxy/config", ResultSuccess)
	}
}

// BenchmarkApplyETag covers the conditional-request helper that replaced the
// inline etag dance. It runs once per panel API request.
func BenchmarkApplyETag(b *testing.B) {
	response := &resty.Response{RawResponse: &http.Response{StatusCode: 200, Header: http.Header{"Etag": []string{"abc"}}}}
	cache := map[string]string{"node": "abc"}
	notModified := ErrNodeNotModified

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ApplyETag(response, cache, "node", notModified)
	}
}
