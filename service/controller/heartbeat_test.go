package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
)

// silenceLogs keeps the monitors' expected error logging out of the test output.
func silenceLogs(t testing.TB) {
	t.Helper()
	previousOut := log.StandardLogger().Out
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousOut) })
}

func TestProbeHost(t *testing.T) {
	cases := map[string]string{
		"":         "127.0.0.1",
		"0.0.0.0":  "127.0.0.1",
		"::":       "::1",
		"[::]":     "::1",
		"::1":      "::1",
		"10.0.0.1": "10.0.0.1",
	}
	for listenIP, expected := range cases {
		assert.Equal(t, expected, probeHost(listenIP), "listen address %q", listenIP)
	}
}

// writeCertificate writes a self-signed certificate valid for the given window
// and returns its path.
func writeCertificate(t testing.TB, validity time.Duration) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "node.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(validity),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "cert.pem")
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(file, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, file.Close())
	return path
}

func TestCertificateDaysRemainingReadsTheLeaf(t *testing.T) {
	days, err := certificateDaysRemaining(writeCertificate(t, 48*time.Hour))
	require.NoError(t, err)
	assert.InDelta(t, 2.0, days, 0.1)
}

func TestCertificateDaysRemainingReportsNegativeForExpiredCertificates(t *testing.T) {
	days, err := certificateDaysRemaining(writeCertificate(t, -24*time.Hour))
	require.NoError(t, err)
	assert.Less(t, days, 0.0)
}

func TestCertificateDaysRemainingRejectsNonCertificatePEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.pem")
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(file, &pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not a certificate")}))
	require.NoError(t, file.Close())

	_, err = certificateDaysRemaining(path)
	assert.ErrorContains(t, err, "no certificate found")
}

func TestCertificateDaysRemainingReportsMissingFile(t *testing.T) {
	_, err := certificateDaysRemaining(filepath.Join(t.TempDir(), "absent.pem"))
	assert.Error(t, err)
}

// closedPort returns a port that was just bound and released, so connecting to
// it is refused.
func closedPort(t testing.TB) uint32 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return uint32(port)
}

// listeningPort returns a port that accepts and immediately closes connections.
func listeningPort(t testing.TB) uint32 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = connection.Close()
		}
	}()
	return uint32(listener.Addr().(*net.TCPAddr).Port)
}

func heartbeatController(t testing.TB, port uint32) *Controller {
	t.Helper()
	users := []api.UserInfo{{UID: 1, Email: "a@example.com"}}
	c := newMonitorTestController(&users)
	c.publishState(&api.NodeInfo{NodeType: "Vmess", NodeID: 1, Port: port}, c.currentUsers(), "Vmess_0.0.0.0_443")
	return c
}

func TestCoreHeartbeatAcceptsAListeningPort(t *testing.T) {
	silenceLogs(t)
	c := heartbeatController(t, listeningPort(t))

	require.NoError(t, c.coreHeartbeatMonitor())
	assert.EqualValues(t, 0, c.coreFailures.Load())
}

// TestCoreHeartbeatEscalatesOnlyAfterRepeatedFailures checks that a single
// unreachable probe is tolerated: a port move briefly leaves nothing listening,
// and restarting the node for that would be worse than the outage.
func TestCoreHeartbeatEscalatesOnlyAfterRepeatedFailures(t *testing.T) {
	silenceLogs(t)
	c := heartbeatController(t, closedPort(t))

	var escalated []string
	previousHook := coreUnhealthy
	coreUnhealthy = func(nodeType, reason string) { escalated = append(escalated, nodeType+" "+reason) }
	t.Cleanup(func() { coreUnhealthy = previousHook })

	for failure := 1; failure < coreProbeFailuresBeforeExit; failure++ {
		require.NoError(t, c.coreHeartbeatMonitor())
		require.Empty(t, escalated, "escalated after %d failure(s)", failure)
		assert.EqualValues(t, failure, c.coreFailures.Load())
	}

	require.NoError(t, c.coreHeartbeatMonitor())
	require.Len(t, escalated, 1)
	assert.Contains(t, escalated[0], "Vmess")
}

// TestCoreHeartbeatResetsAfterRecovery checks that a successful probe clears the
// failure streak, so an intermittent outage cannot accumulate into a restart.
func TestCoreHeartbeatResetsAfterRecovery(t *testing.T) {
	silenceLogs(t)
	c := heartbeatController(t, closedPort(t))

	require.NoError(t, c.coreHeartbeatMonitor())
	require.EqualValues(t, 1, c.coreFailures.Load())

	c.publishState(&api.NodeInfo{NodeType: "Vmess", NodeID: 1, Port: listeningPort(t)}, c.currentUsers(), "Vmess_0.0.0.0_443")
	require.NoError(t, c.coreHeartbeatMonitor())
	assert.EqualValues(t, 0, c.coreFailures.Load())
}

func TestCoreHeartbeatCanBeDisabled(t *testing.T) {
	silenceLogs(t)
	c := heartbeatController(t, closedPort(t))
	c.config.DisableCoreHeartbeat = true

	require.NoError(t, c.coreHeartbeatMonitor())
	assert.EqualValues(t, 0, c.coreFailures.Load())
}
