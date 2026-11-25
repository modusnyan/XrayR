package newV2board

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

// panelStub answers every request with the given status, optionally adding the
// panel's "node disabled" header.
func panelStub(t *testing.T, status int, disabledHeader bool) *APIClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if disabledHeader {
			w.Header().Set(api.NodeStateHeader, api.NodeStateDisabled)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"fail","message":"Server is disabled","data":null,"error":null}`))
	}))
	t.Cleanup(server.Close)
	client := New(&api.Config{APIHost: server.URL, Key: "token", NodeID: 7, NodeType: "Vless", Timeout: 2})
	client.Client.SetRetryCount(0)
	return client
}

func TestDisabledNodeIsReportedAsErrNodeDisabled(t *testing.T) {
	client := panelStub(t, http.StatusGone, true)
	client.eTags["node"] = `"stale-node"`
	client.eTags["users"] = `"stale-users"`

	_, err := client.GetNodeInfo()
	if !errors.Is(err, api.ErrNodeDisabled) {
		t.Fatalf("GetNodeInfo: want ErrNodeDisabled, got %v", err)
	}
	if len(client.eTags) != 0 {
		t.Fatalf("ETags must be forgotten once the node is disabled, got %v", client.eTags)
	}

	client.eTags["users"] = `"stale-users"`
	_, err = client.GetUserList()
	if !errors.Is(err, api.ErrNodeDisabled) {
		t.Fatalf("GetUserList: want ErrNodeDisabled, got %v", err)
	}
	if len(client.eTags) != 0 {
		t.Fatalf("ETags must be forgotten once the node is disabled, got %v", client.eTags)
	}
}

func TestOnlyTheFullSignalMeansDisabled(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header bool
	}{
		{"410 without header", http.StatusGone, false},
		{"other error with header", http.StatusNotFound, true},
		{"server error", http.StatusInternalServerError, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := panelStub(t, tc.status, tc.header)
			client.eTags["node"] = `"kept"`

			_, err := client.GetNodeInfo()
			if err == nil {
				t.Fatal("want an error")
			}
			if errors.Is(err, api.ErrNodeDisabled) {
				t.Fatalf("%s must not be treated as a disabled node: %v", tc.name, err)
			}
			if client.eTags["node"] != `"kept"` {
				t.Fatal("ETags must survive ordinary panel errors")
			}
		})
	}
}
