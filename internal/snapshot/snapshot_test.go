package snapshot

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
)

func testClientInfo() api.ClientInfo {
	return api.ClientInfo{APIHost: "https://panel.example.com", NodeID: 12, NodeType: "Vless"}
}

func testNode() *api.NodeInfo {
	return &api.NodeInfo{
		NodeType: "Vless",
		NodeID:   12,
		Port:     443,
		REALITYConfig: &api.REALITYConfig{
			Dest:       "www.example.com:443",
			PrivateKey: "super-secret-private-key",
		},
	}
}

func testUsers() *[]api.UserInfo {
	return &[]api.UserInfo{{UID: 1, Email: "a@example.com", UUID: "uuid-1"}}
}

func testRules() *[]api.DetectRule {
	return &[]api.DetectRule{{ID: 5, Pattern: regexp.MustCompile(`blocked\.example`)}, {ID: 6}}
}

func newStore(t *testing.T, maxAge time.Duration) *Store {
	t.Helper()
	return New(t.TempDir(), testClientInfo(), maxAge)
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), testUsers(), testRules()))

	node, users, rules, savedAt, err := store.Load()

	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, "Vless", node.NodeType)
	assert.Equal(t, uint32(443), node.Port)
	require.Len(t, *users, 1)
	assert.Equal(t, "a@example.com", (*users)[0].Email)
	require.Len(t, *rules, 1, "rules without a pattern are dropped")
	assert.Equal(t, 5, (*rules)[0].ID)
	assert.Equal(t, `blocked\.example`, (*rules)[0].Pattern.String())
	assert.False(t, savedAt.IsZero())
}

func TestSaveRequiresNodeAndUsers(t *testing.T) {
	store := newStore(t, time.Hour)

	assert.Error(t, store.Save(nil, testUsers(), nil))
	assert.Error(t, store.Save(testNode(), nil, nil))
}

func TestSaveNeverPersistsRealityPrivateKey(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), testUsers(), nil))

	data, err := os.ReadFile(store.path())
	require.NoError(t, err)
	assert.NotContains(t, string(data), "super-secret-private-key")

	node, _, _, _, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, node.REALITYConfig)
	assert.Empty(t, node.REALITYConfig.PrivateKey)
	assert.Equal(t, "www.example.com:443", node.REALITYConfig.Dest)
}

func TestLoadRejectsExpiredSnapshot(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), testUsers(), nil))

	expired := New(store.directory, testClientInfo(), time.Nanosecond)
	time.Sleep(time.Millisecond)

	_, _, _, _, err := expired.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestLoadIgnoresMaxAgeWhenDisabled(t *testing.T) {
	store := newStore(t, 0)
	require.NoError(t, store.Save(testNode(), testUsers(), nil))

	_, _, _, _, err := store.Load()
	assert.NoError(t, err)
}

func TestLoadRejectsTamperedChecksum(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), testUsers(), nil))

	data, err := os.ReadFile(store.path())
	require.NoError(t, err)
	var payload Payload
	require.NoError(t, json.Unmarshal(data, &payload))
	payload.Users[0].Email = "attacker@example.com"
	tampered, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.path(), tampered, 0o600))

	_, _, _, _, err = store.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")
}

func TestLoadRejectsSnapshotFromAnotherNode(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), testUsers(), nil))

	other := New(store.directory, api.ClientInfo{APIHost: "https://panel.example.com", NodeID: 99, NodeType: "Vless"}, time.Hour)

	_, _, _, _, err := other.Load()
	require.Error(t, err)
	// The message differs per platform ("no such file or directory" on Linux,
	// "The system cannot find the file specified." on Windows); only the os
	// error kind matters: another node's file simply does not exist.
	assert.ErrorIs(t, err, os.ErrNotExist, "each node has its own snapshot file")
}

func TestLoadRejectsIncompleteSnapshot(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(&api.NodeInfo{NodeType: "Vless", NodeID: 12, Port: 0}, testUsers(), nil))

	_, _, _, _, err := store.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestSaveAndLoadEmptyUserList(t *testing.T) {
	store := newStore(t, time.Hour)
	require.NoError(t, store.Save(testNode(), &[]api.UserInfo{}, nil))

	data, err := os.ReadFile(store.path())
	require.NoError(t, err)
	assert.Contains(t, string(data), `"users": []`, "an empty list is persisted as an array, not null")

	node, users, _, _, err := store.Load()
	require.NoError(t, err, "a node whose users were all removed must still restart from its snapshot")
	require.NotNil(t, node)
	require.NotNil(t, users)
	assert.Empty(t, *users)
}

func TestLoadRejectsNullUserList(t *testing.T) {
	store := newStore(t, time.Hour)
	payload := Payload{Version: Version, Identity: store.identity, SavedAt: time.Now().UTC(), Node: *testNode(), Users: nil}
	checksum, err := payloadChecksum(payload)
	require.NoError(t, err)
	payload.Checksum = checksum
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(store.directory, 0o700))
	require.NoError(t, os.WriteFile(store.path(), data, 0o600))

	_, _, _, _, err = store.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestIdentityIsStableAcrossHostCaseAndPath(t *testing.T) {
	upper := Identity(api.ClientInfo{APIHost: "https://Panel.Example.com/api", NodeID: 12, NodeType: "VLESS"})
	lower := Identity(api.ClientInfo{APIHost: "https://panel.example.com", NodeID: 12, NodeType: "vless"})

	assert.Equal(t, lower, upper)
}
