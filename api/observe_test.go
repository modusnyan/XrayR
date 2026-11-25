package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObservePanelRequestPublishesTheCounter proves the counter is reachable
// through the default registry, which is what the /metrics endpoint serves.
// Without the instrumentation call the series never appears, so an empty
// xrayr_panel_requests_total is the symptom this guards against.
func TestObservePanelRequestPublishesTheCounter(t *testing.T) {
	ObservePanelRequest("Xboard", "/observe-test", ResultSuccess)

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "xrayr_panel_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["panel"] == "Xboard" && labels["operation"] == "/observe-test" && labels["result"] == ResultSuccess {
				assert.Equal(t, float64(1), metric.GetCounter().GetValue())
				return
			}
		}
	}
	t.Fatal("xrayr_panel_requests_total did not expose the recorded call")
}

func TestPanelResultLabelUsesTheClassifiedKind(t *testing.T) {
	cases := map[int]string{
		401: string(ErrorAuthentication),
		403: string(ErrorAuthentication),
		404: string(ErrorNotFound),
		429: string(ErrorRateLimited),
		400: string(ErrorInvalidPayload),
		500: string(ErrorServer),
		503: string(ErrorServer),
	}
	for statusCode, expected := range cases {
		err := ClassifyError("/op", "Xboard", 1, statusCode, nil)
		assert.Equal(t, expected, panelResultLabel(err), "HTTP %d", statusCode)
	}
}

func TestPanelResultLabelFallsBackToServerForUnclassifiedErrors(t *testing.T) {
	assert.Equal(t, string(ErrorServer), panelResultLabel(errors.New("plain error")))
	assert.Equal(t, string(ErrorServer), panelResultLabel(fmt.Errorf("wrapped: %w", errors.New("plain error"))))
}

func TestPanelResultLabelKeepsTheKindThroughWrapping(t *testing.T) {
	classified := ClassifyError("/op", "Xboard", 1, 429, nil)
	wrapped := fmt.Errorf("panel call failed: %w", classified)
	assert.Equal(t, string(ErrorRateLimited), panelResultLabel(wrapped))
}
