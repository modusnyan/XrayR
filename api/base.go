package api

import "github.com/go-resty/resty/v2"

// BaseClient carries the identity fields every panel adapter shares and
// implements the boilerplate half of the Client interface.
//
// Adapters embed it and keep issuing requests through Client; Debug is provided
// so the common case needs no per-adapter implementation.
type BaseClient struct {
	Client   *resty.Client
	APIHost  string
	NodeID   int
	Key      string
	NodeType string
}

// NewBaseClient builds the shared client state from the node configuration.
// The returned client is the one produced by NewHTTPClient, so adapters can
// still attach their own credentials and headers to it.
func NewBaseClient(apiConfig *Config) *BaseClient {
	return &BaseClient{
		Client:   NewHTTPClient(apiConfig),
		APIHost:  apiConfig.APIHost,
		NodeID:   apiConfig.NodeID,
		Key:      apiConfig.Key,
		NodeType: apiConfig.NodeType,
	}
}

// Describe returns a description of the client.
func (b *BaseClient) Describe() ClientInfo {
	return ClientInfo{APIHost: b.APIHost, NodeID: b.NodeID, Key: b.Key, NodeType: b.NodeType}
}

// Debug enables request/response logging on the underlying HTTP client.
func (b *BaseClient) Debug() {
	b.Client.SetDebug(true)
}
