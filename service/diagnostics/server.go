package diagnostics

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type NodeStatus struct {
	Panel     string    `json:"panel"`
	NodeID    int       `json:"node_id"`
	NodeType  string    `json:"node_type"`
	Ready     bool      `json:"ready"`
	Users     int       `json:"users"`
	LastSync  time.Time `json:"last_sync,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type Provider interface{ DiagnosticStatus() []NodeStatus }

type Server struct {
	address  string
	provider Provider
	server   *http.Server
	mu       sync.Mutex
	listener net.Listener
}

func New(address string, provider Provider) *Server {
	return &Server{address: address, provider: provider}
}
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/readyz", s.ready)
	mux.HandleFunc("/status", s.status)
	mux.Handle("/metrics", promhttp.Handler())
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = listener
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.server.Serve(listener) }()
	return nil
}
func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	err := s.server.Shutdown(ctx)
	s.server = nil
	s.listener = nil
	return err
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	statuses := s.provider.DiagnosticStatus()
	ready := len(statuses) > 0
	for _, status := range statuses {
		ready = ready && status.Ready
	}
	code := http.StatusOK
	if !ready {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ready": ready})
}
func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.provider.DiagnosticStatus())
}
