package api

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// NewHTTPClient builds the resty client shared by every panel adapter: a bounded
// timeout, three retries and a transport-error logger. Adapters add their own
// credentials, headers and per-request options on top; the base URL is taken from
// the API configuration.
func NewHTTPClient(apiConfig *Config) *resty.Client {
	client := resty.New()
	client.SetRetryCount(3)
	if apiConfig.Timeout > 0 {
		client.SetTimeout(time.Duration(apiConfig.Timeout) * time.Second)
	} else {
		client.SetTimeout(5 * time.Second)
	}
	client.OnError(func(req *resty.Request, err error) {
		var responseErr *resty.ResponseError
		if errors.As(err, &responseErr) {
			// responseErr.Response holds the last response from the server and
			// responseErr.Err the original transport error.
			log.Print(responseErr.Err)
		}
	})
	client.SetBaseURL(apiConfig.APIHost)
	return client
}

// ReadLocalRuleList loads an operator-provided audit rule file: one regular
// expression per line, blank lines and '#' comments ignored.
//
// An unreadable file or an unparsable line is reported and skipped. Neither may
// terminate the process, because this runs during adapter construction and a
// typo in a local file must not take the whole node down.
func ReadLocalRuleList(path string) []DetectRule {
	rules := make([]DetectRule, 0)
	if path == "" {
		return rules
	}

	file, err := os.Open(path)
	if err != nil {
		log.Printf("Failed to open local rule list %s: %s", path, err)
		return rules
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Error when closing rule list %s: %s", path, closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = AppendRule(rules, -1, line)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error while reading rule list %s: %s", path, err)
	}
	return rules
}

// NodeStateHeader and NodeStateDisabled form the panel's "node disabled" signal.
// Xboard answers config/user requests for a switched-off node with HTTP 410 and
// this header; both must be present, so an unrelated 410 from a proxy or CDN is
// never mistaken for it.
const (
	NodeStateHeader   = "X-Node-State"
	NodeStateDisabled = "disabled"
)

// IsNodeDisabledResponse reports whether res is the panel's "node disabled" signal.
func IsNodeDisabledResponse(res *resty.Response) bool {
	return res != nil &&
		res.StatusCode() == 410 &&
		strings.EqualFold(strings.TrimSpace(res.Header().Get(NodeStateHeader)), NodeStateDisabled)
}

// NodeDisabledError wraps ErrNodeDisabled in an APIError so it is both
// classified for metrics and matchable with errors.Is(err, ErrNodeDisabled).
func NodeDisabledError(path, panel string, nodeID int) error {
	return &APIError{Kind: ErrorNodeDisabled, Operation: path, Panel: panel, NodeID: nodeID, StatusCode: 410, Err: ErrNodeDisabled}
}

// CheckResponse validates the transport-level outcome of a panel request before
// the adapter inspects the response body.
func CheckResponse(res *resty.Response, path, panel string, nodeID int, err error) error {
	statusCode := 0
	if res != nil {
		statusCode = res.StatusCode()
	}
	if err != nil || statusCode >= 400 {
		return ClassifyError(path, panel, nodeID, statusCode, err)
	}
	return nil
}
