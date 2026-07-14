package api

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

type ErrorKind string

const (
	ErrorNetwork        ErrorKind = "network"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorAuthentication ErrorKind = "authentication"
	ErrorNotFound       ErrorKind = "not_found"
	ErrorRateLimited    ErrorKind = "rate_limited"
	ErrorInvalidPayload ErrorKind = "invalid_payload"
	ErrorServer         ErrorKind = "server"
)

// APIError provides stable machine-readable error classification without
// exposing query parameters or response secrets.
type APIError struct {
	Kind       ErrorKind
	Operation  string
	Panel      string
	NodeID     int
	StatusCode int
	Err        error
}

func (e *APIError) Error() string {
	message := fmt.Sprintf("panel %s operation %s failed", e.Panel, e.Operation)
	if e.NodeID > 0 {
		message += fmt.Sprintf(" for node %d", e.NodeID)
	}
	if e.StatusCode > 0 {
		message += fmt.Sprintf(" (HTTP %d)", e.StatusCode)
	}
	if e.Err != nil {
		message += ": " + sanitizeError(e.Err.Error())
	}
	return message
}
func (e *APIError) Unwrap() error { return e.Err }

func ClassifyError(operation, panel string, nodeID, statusCode int, err error) error {
	if err == nil && statusCode < 400 {
		return nil
	}
	kind := ErrorServer
	if statusCode == 401 || statusCode == 403 {
		kind = ErrorAuthentication
	} else if statusCode == 404 {
		kind = ErrorNotFound
	} else if statusCode == 429 {
		kind = ErrorRateLimited
	} else if statusCode >= 400 && statusCode < 500 {
		kind = ErrorInvalidPayload
	} else if statusCode >= 500 {
		kind = ErrorServer
	} else if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				kind = ErrorTimeout
			} else {
				kind = ErrorNetwork
			}
		}
	}
	return &APIError{Kind: kind, Operation: operation, Panel: panel, NodeID: nodeID, StatusCode: statusCode, Err: err}
}

func sanitizeError(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.User = nil
		return parsed.String()
	}
	if index := len(value); index > 0 {
		for i, char := range value {
			if char == '?' {
				return value[:i]
			}
		}
	}
	return value
}
