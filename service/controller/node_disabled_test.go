package controller

import (
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

// disabledClient is a panel that reports the node as disabled.
type disabledClient struct{ stubClient }

func (d *disabledClient) GetNodeInfo() (*api.NodeInfo, error) {
	return nil, api.NodeDisabledError("/config", "test", 1)
}

func (d *disabledClient) GetUserList() (*[]api.UserInfo, error) {
	return nil, api.NodeDisabledError("/user", "test", 1)
}

// newDisabledTestController returns a controller whose node has no inbound tag,
// so taking it down needs no live xray-core instance.
func newDisabledTestController(t *testing.T, users *[]api.UserInfo) *Controller {
	t.Helper()
	silenceLogs(t)
	c := newMonitorTestController(users)
	node := c.currentNode()
	c.apiClient = &disabledClient{stubClient{node: node, users: users}}
	c.publishState(node, users, "")
	return c
}

func TestNodeMonitorTakesTheNodeDownWhenThePanelDisablesIt(t *testing.T) {
	users := []api.UserInfo{{UID: 1, Email: "a@example.com"}}
	c := newDisabledTestController(t, &users)

	if err := c.nodeInfoMonitor(); err != nil {
		t.Fatalf("nodeInfoMonitor returned error: %v", err)
	}
	if !c.nodeDisabled.Load() {
		t.Fatal("node not marked disabled")
	}
	node, kept, _ := c.runtimeSnapshot()
	if node != nil {
		t.Fatal("node still published after being disabled")
	}
	if kept == nil || len(*kept) != 1 {
		t.Fatal("users must be kept until their remaining traffic is reported")
	}
	if status := c.DiagnosticStatus(); status.Ready || status.LastError == "" {
		t.Fatalf("diagnostics must show the node offline: %+v", status)
	}

	// Further rounds while disabled are no-ops.
	if err := c.nodeInfoMonitor(); err != nil {
		t.Fatalf("second nodeInfoMonitor returned error: %v", err)
	}
}

func TestUserMonitorClearsTheDisabledNodeAfterReporting(t *testing.T) {
	// An empty list keeps the user monitor away from the stats manager.
	users := []api.UserInfo{}
	c := newDisabledTestController(t, &users)
	c.takeNodeDown()

	if err := c.userInfoMonitor(); err != nil {
		t.Fatalf("userInfoMonitor returned error: %v", err)
	}
	if _, cleared, tag := c.runtimeSnapshot(); cleared != nil || tag != "" {
		t.Fatalf("disabled node state not cleared: users=%v tag=%q", cleared, tag)
	}
}

func TestUserMonitorLeavesAnActiveNodeAlone(t *testing.T) {
	silenceLogs(t)
	users := []api.UserInfo{}
	c := newMonitorTestController(&users)

	if err := c.userInfoMonitor(); err != nil {
		t.Fatalf("userInfoMonitor returned error: %v", err)
	}
	if node, kept, tag := c.runtimeSnapshot(); node == nil || kept == nil || tag == "" {
		t.Fatal("an active node's state must not be cleared")
	}
}
