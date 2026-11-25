package controller

import (
	"io"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/XrayR-project/XrayR/api"
)

// stubClient is a minimal api.Client used by the concurrency regression tests.
type stubClient struct {
	node  *api.NodeInfo
	users *[]api.UserInfo
}

func (s *stubClient) GetNodeInfo() (*api.NodeInfo, error)   { return s.node, nil }
func (s *stubClient) GetUserList() (*[]api.UserInfo, error) { return s.users, nil }
func (s *stubClient) Debug()                                {}
func (s *stubClient) Describe() api.ClientInfo {
	return api.ClientInfo{APIHost: "http://stub.invalid", NodeType: s.node.NodeType, NodeID: s.node.NodeID}
}

// newMonitorTestController builds a Controller whose monitor tasks can run without
// a live xray-core instance: an unchanged node, an empty user list, and every panel
// capability disabled so the monitors only touch their in-memory state.
func newMonitorTestController(users *[]api.UserInfo) *Controller {
	node := &api.NodeInfo{NodeType: "Vmess", NodeID: 1, Port: 443}
	c := &Controller{
		config: &Config{
			UpdatePeriodic: 1,
			DisableGetRule: true,
			AutoSpeedLimitConfig: &AutoSpeedLimitConfig{
				Limit: 0,
			},
		},
		apiClient:    &stubClient{node: node, users: users},
		capabilities: api.PanelCapabilities{},
		startAt:      time.Now().Add(-time.Hour),
		logger:       log.NewEntry(log.StandardLogger()),
	}
	c.publishState(node, users, "Vmess_0.0.0.0_443")
	return c
}

// TestMonitorStateAccessIsRaceFree runs nodeInfoMonitor and userInfoMonitor
// concurrently. Start() launches both with the same UpdatePeriodic interval, so
// they genuinely overlap in production.
//
// Run with `go test -race`. While nodeInfo/userList/Tag were plain fields this test
// reported a data race between the write of the user list in nodeInfoMonitor and
// the read of it in userInfoMonitor.
func TestMonitorStateAccessIsRaceFree(t *testing.T) {
	previousOut := log.StandardLogger().Out
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousOut)

	users := []api.UserInfo{}
	c := newMonitorTestController(&users)

	var wg sync.WaitGroup
	deadline := time.Now().Add(2 * time.Second)

	wg.Add(2)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			if err := c.nodeInfoMonitor(); err != nil {
				t.Errorf("nodeInfoMonitor returned error: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			if err := c.userInfoMonitor(); err != nil {
				t.Errorf("userInfoMonitor returned error: %v", err)
				return
			}
		}
	}()
	wg.Wait()
}

// TestConcurrentStatePublication checks that the atomic runtime state snapshot can
// be published and read concurrently without a torn read.
func TestConcurrentStatePublication(t *testing.T) {
	previousOut := log.StandardLogger().Out
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousOut)

	users := []api.UserInfo{{UID: 1, Email: "a@example.com"}}
	c := newMonitorTestController(&users)
	replacement := []api.UserInfo{{UID: 2, Email: "b@example.com"}}

	var wg sync.WaitGroup
	deadline := time.Now().Add(time.Second)

	wg.Add(2)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			c.publishUsers(&replacement)
		}
	}()
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			_, current, tag := c.runtimeSnapshot()
			if current == nil || tag == "" {
				t.Error("runtimeSnapshot returned zero values")
				return
			}
			for _, user := range *current {
				_ = buildUserTag(tag, &user)
			}
		}
	}()
	wg.Wait()
}
