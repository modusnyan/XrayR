package newV2board

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

// recordingPanel answers every request with status and remembers the last
// request path and body.
type recordingPanel struct {
	status   int
	requests atomic.Int32
	path     atomic.Value
	body     atomic.Value
}

func newRecordingPanel(t *testing.T, status int) (*recordingPanel, *APIClient) {
	t.Helper()
	panel := &recordingPanel{status: status}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panel.requests.Add(1)
		panel.path.Store(r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		panel.body.Store(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(panel.status)
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	t.Cleanup(server.Close)
	client := New(&api.Config{APIHost: server.URL, Key: "token", NodeID: 9, NodeType: "Vless", Timeout: 2})
	client.Client.SetRetryCount(0)
	return panel, client
}

func (p *recordingPanel) decoded(t *testing.T) map[string]any {
	t.Helper()
	var body map[string]any
	raw, _ := p.body.Load().([]byte)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("request body is not a JSON object: %v (%s)", err, raw)
	}
	return body
}

func TestOnlineUsersAreGroupedByUser(t *testing.T) {
	panel, client := newRecordingPanel(t, http.StatusOK)
	online := []api.OnlineUser{{UID: 1, IP: "1.1.1.1"}, {UID: 1, IP: "2.2.2.2"}, {UID: 7, IP: "3.3.3.3"}}
	if err := client.ReportNodeOnlineUsers(&online); err != nil {
		t.Fatal(err)
	}
	if path := panel.path.Load(); path != "/api/v1/server/UniProxy/alive" {
		t.Fatalf("posted to %v", path)
	}
	body := panel.decoded(t)
	if ips, _ := body["1"].([]any); len(ips) != 2 {
		t.Errorf("user 1 IPs = %v", body["1"])
	}
	if ips, _ := body["7"].([]any); len(ips) != 1 {
		t.Errorf("user 7 IPs = %v", body["7"])
	}
}

func TestEmptyOnlineReportIsStillSent(t *testing.T) {
	panel, client := newRecordingPanel(t, http.StatusOK)
	if err := client.ReportNodeOnlineUsers(&[]api.OnlineUser{}); err != nil {
		t.Fatal(err)
	}
	if panel.requests.Load() != 1 || len(panel.decoded(t)) != 0 {
		t.Fatalf("an empty report must be posted as {} so the panel count drops to zero")
	}
}

func TestNodeStatusCarriesLoadAndSyncState(t *testing.T) {
	panel, client := newRecordingPanel(t, http.StatusOK)
	err := client.ReportNodeStatus(&api.NodeStatus{
		CPU: 12.5, MemTotal: 2048, MemUsed: 1024, DiskTotal: 100, DiskUsed: 40, Uptime: 3600,
		Users: 5, SyncError: "unsupported shadowsocks plugin", LastSyncAt: 1700000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if path := panel.path.Load(); path != "/api/v1/server/UniProxy/status" {
		t.Fatalf("posted to %v", path)
	}
	body := panel.decoded(t)
	if mem, _ := body["mem"].(map[string]any); mem["total"] != float64(2048) || mem["used"] != float64(1024) {
		t.Errorf("mem = %v", body["mem"])
	}
	sync, _ := body["sync"].(map[string]any)
	if sync["ok"] != false || sync["error"] != "unsupported shadowsocks plugin" || sync["last_sync_at"] != float64(1700000000) {
		t.Errorf("sync = %v", sync)
	}
}

func TestHealthyNodeStatusSendsNullError(t *testing.T) {
	panel, client := newRecordingPanel(t, http.StatusOK)
	if err := client.ReportNodeStatus(&api.NodeStatus{}); err != nil {
		t.Fatal(err)
	}
	sync, _ := panel.decoded(t)["sync"].(map[string]any)
	if sync["ok"] != true || sync["error"] != nil {
		t.Errorf("sync = %v", sync)
	}
}

func TestReportsStopWhenThePanelHasNoEndpoint(t *testing.T) {
	panel, client := newRecordingPanel(t, http.StatusNotFound)
	for i := 0; i < 3; i++ {
		if err := client.ReportNodeStatus(&api.NodeStatus{}); err != nil {
			t.Fatalf("a missing endpoint must not be an error: %v", err)
		}
		if err := client.ReportNodeOnlineUsers(&[]api.OnlineUser{}); err != nil {
			t.Fatalf("a missing endpoint must not be an error: %v", err)
		}
	}
	if got := panel.requests.Load(); got != 2 {
		t.Fatalf("panel was asked %d times, want once per endpoint", got)
	}
}

func TestReportErrorsAreStillReported(t *testing.T) {
	_, client := newRecordingPanel(t, http.StatusInternalServerError)
	if err := client.ReportNodeStatus(&api.NodeStatus{}); err == nil {
		t.Fatal("a 500 must be reported")
	}
	if client.statusUnsupported.Load() {
		t.Fatal("a 500 must not switch the report off")
	}
}
