package service

import "github.com/XrayR-project/XrayR/service/diagnostics"

// Service is the interface of all the services running in the panel
type Service interface {
	Start() error
	Close() error
	Restart
}

// StatusProvider exposes sanitized runtime state for diagnostics.
type StatusProvider interface {
	DiagnosticStatus() diagnostics.NodeStatus
}

// Restart the service
type Restart interface {
	Start() error
	Close() error
}
