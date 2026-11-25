package controller

import (
	"errors"
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

// scriptedNodeClient answers GetNodeInfo from a queue; once the queue is empty
// it keeps answering 304.
type scriptedNodeClient struct {
	stubClient
	answers []func() (*api.NodeInfo, error)
}

func (s *scriptedNodeClient) GetNodeInfo() (*api.NodeInfo, error) {
	if len(s.answers) == 0 {
		return nil, api.ErrNodeNotModified
	}
	next := s.answers[0]
	s.answers = s.answers[1:]
	return next()
}

func newScriptedController(t *testing.T, answers ...func() (*api.NodeInfo, error)) *Controller {
	t.Helper()
	silenceLogs(t)
	users := []api.UserInfo{}
	c := newMonitorTestController(&users)
	c.apiClient = &scriptedNodeClient{stubClient: stubClient{node: c.currentNode(), users: &users}, answers: answers}
	return c
}

func runNodeMonitor(t *testing.T, c *Controller) string {
	t.Helper()
	if err := c.nodeInfoMonitor(); err != nil {
		t.Fatalf("nodeInfoMonitor: %v", err)
	}
	return c.DiagnosticStatus().LastError
}

func TestRejectedConfigStaysReportedAcrossNotModifiedRounds(t *testing.T) {
	rejected := errors.New("unsupported shadowsocks plugin \"v2ray-plugin\"")
	c := newScriptedController(t, func() (*api.NodeInfo, error) { return nil, rejected })

	if got := runNodeMonitor(t, c); got == "" {
		t.Fatal("the rejection was not recorded")
	}
	// The panel keeps the rejected config and answers 304 from now on.
	for i := 0; i < 2; i++ {
		if got := runNodeMonitor(t, c); got == "" {
			t.Fatalf("round %d: a 304 for the rejected config cleared the error", i+1)
		}
	}
}

func TestRejectionClearsWhenThePanelReturnsToTheActiveConfig(t *testing.T) {
	var c *Controller
	c = newScriptedController(t,
		func() (*api.NodeInfo, error) { return nil, errors.New("server port must > 0") },
		func() (*api.NodeInfo, error) { node := *c.currentNode(); return &node, nil },
	)
	runNodeMonitor(t, c)
	if got := runNodeMonitor(t, c); got != "" {
		t.Fatalf("error kept after the panel went back to the running config: %q", got)
	}
}

func TestTransientPanelErrorsDoNotStick(t *testing.T) {
	c := newScriptedController(t, func() (*api.NodeInfo, error) {
		return nil, api.ClassifyError("/config", "test", 1, 502, errors.New("bad gateway"))
	})
	if got := runNodeMonitor(t, c); got == "" {
		t.Fatal("the failure was not recorded")
	}
	if got := runNodeMonitor(t, c); got != "" {
		t.Fatalf("a passing panel outage must clear on the next good round, got %q", got)
	}
}

func TestNodeStatusCarriesTheSyncState(t *testing.T) {
	silenceLogs(t)
	users := []api.UserInfo{{UID: 1, Email: "a@example.com"}, {UID: 2, Email: "b@example.com"}}
	c := newMonitorTestController(&users)

	healthy := c.nodeStatus(&users)
	if healthy.SyncError != "" || healthy.Users != 2 || healthy.Goroutines == 0 {
		t.Fatalf("healthy status = %+v", healthy)
	}

	c.recordRejectedConfig(errors.New("bad config"))
	c.markSynced()
	if status := c.nodeStatus(&users); status.SyncError != "bad config" || status.LastSyncAt == 0 {
		t.Fatalf("status after a rejection = %+v", status)
	}
}
