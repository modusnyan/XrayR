package api

import (
	"errors"

	"github.com/XrayR-project/XrayR/observability"
)

// ResultSuccess labels a panel API call that completed and parsed.
const ResultSuccess = "success"

// ObservePanelRequest records the outcome of one panel API call.
//
// Adapters call it exactly once per request, after the response has been fully
// handled, so xrayr_panel_requests_total reflects logical outcomes rather than
// transport events: a 200 whose body fails to parse is reported as a failure,
// not as a success.
func ObservePanelRequest(panel, operation, result string) {
	observability.PanelRequests.WithLabelValues(panel, operation, result).Inc()
}

// ObservePanelError records a failed panel API call, deriving the result label
// from the classified error so the counter stays consistent with the error the
// caller actually receives.
func ObservePanelError(panel, operation string, err error) {
	ObservePanelRequest(panel, operation, panelResultLabel(err))
}

// panelResultLabel maps a panel error to the result label used by
// xrayr_panel_requests_total. An error that was never classified is reported as
// a server failure, matching ClassifyError's default.
func panelResultLabel(err error) string {
	kind := ErrorServer
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		kind = apiErr.Kind
	}
	return string(kind)
}
