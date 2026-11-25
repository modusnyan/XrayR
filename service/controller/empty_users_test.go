package controller

import (
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

func usersOf(uids ...int) *[]api.UserInfo {
	list := make([]api.UserInfo, len(uids))
	for i, uid := range uids {
		list[i] = api.UserInfo{UID: uid, Email: "u@example.com"}
	}
	return &list
}

func TestValidateRuntimeSnapshotAcceptsAnEmptyUserList(t *testing.T) {
	node := &api.NodeInfo{NodeType: "Vmess", NodeID: 1, Port: 443}
	if err := validateRuntimeSnapshot(node, &[]api.UserInfo{}); err != nil {
		t.Fatalf("empty user list rejected: %v", err)
	}
	if err := validateRuntimeSnapshot(node, nil); err == nil {
		t.Fatal("nil user list accepted")
	}
}

func TestEmptyUserListIsHeldBackUntilConfirmed(t *testing.T) {
	silenceLogs(t)
	c := newMonitorTestController(usersOf(1, 2))

	users, changed := c.confirmUserList(&[]api.UserInfo{}, false)
	if changed || len(*users) != 2 {
		t.Fatalf("first empty list applied: changed=%v users=%d", changed, len(*users))
	}

	users, changed = c.confirmUserList(&[]api.UserInfo{}, false)
	if !changed || len(*users) != 0 {
		t.Fatalf("second empty list not applied: changed=%v users=%d", changed, len(*users))
	}
	if c.emptyUsersPending.Load() {
		t.Fatal("pending flag not cleared after confirmation")
	}
}

func TestEmptyUserListIsConfirmedByNotModified(t *testing.T) {
	silenceLogs(t)
	c := newMonitorTestController(usersOf(1))

	if _, changed := c.confirmUserList(&[]api.UserInfo{}, false); changed {
		t.Fatal("first empty list applied")
	}
	// The next pull answers 304 against the empty list's ETag.
	users, changed := c.confirmUserList(nil, true)
	if !changed || users == nil || len(*users) != 0 {
		t.Fatalf("304 did not confirm the empty list: changed=%v users=%v", changed, users)
	}
}

func TestNonEmptyUserListCancelsThePendingConfirmation(t *testing.T) {
	silenceLogs(t)
	c := newMonitorTestController(usersOf(1))

	c.confirmUserList(&[]api.UserInfo{}, false)
	users, changed := c.confirmUserList(usersOf(1, 3), false)
	if !changed || len(*users) != 2 {
		t.Fatalf("non-empty list not applied: changed=%v users=%d", changed, len(*users))
	}
	if c.emptyUsersPending.Load() {
		t.Fatal("pending flag survived a non-empty list")
	}
	// A later single empty list must be held back again.
	c.publishUsers(users)
	if _, changed := c.confirmUserList(&[]api.UserInfo{}, false); changed {
		t.Fatal("empty list applied without confirmation")
	}
}

func TestNotModifiedWithoutPendingKeepsCurrentUsers(t *testing.T) {
	silenceLogs(t)
	c := newMonitorTestController(usersOf(1, 2))

	users, changed := c.confirmUserList(nil, true)
	if changed || len(*users) != 2 {
		t.Fatalf("304 changed users: changed=%v users=%d", changed, len(*users))
	}
}

func TestEmptyUserListOnAnEmptyNodeAppliesImmediately(t *testing.T) {
	silenceLogs(t)
	c := newMonitorTestController(&[]api.UserInfo{})

	users, changed := c.confirmUserList(&[]api.UserInfo{}, false)
	if !changed || len(*users) != 0 {
		t.Fatalf("empty list on empty node: changed=%v users=%d", changed, len(*users))
	}
	if c.emptyUsersPending.Load() {
		t.Fatal("pending flag set on an already empty node")
	}
}
