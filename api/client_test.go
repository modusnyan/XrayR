package api_test

import (
	"context"
	stderrors "errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
)

func writeRuleFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rulelist")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestReadLocalRuleListParsesOnePatternPerLine(t *testing.T) {
	path := writeRuleFile(t, "# leading comment\n\n   \nfoo\\.bar\nbaz\n")

	rules := api.ReadLocalRuleList(path)

	require.Len(t, rules, 2)
	assert.Equal(t, -1, rules[0].ID)
	assert.Equal(t, `foo\.bar`, rules[0].Pattern.String())
	assert.Equal(t, "baz", rules[1].Pattern.String())
}

func TestReadLocalRuleListSkipsInvalidPatternsInsteadOfPanicking(t *testing.T) {
	path := writeRuleFile(t, "valid\n(unclosed\n")

	rules := api.ReadLocalRuleList(path)

	require.Len(t, rules, 1)
	assert.Equal(t, "valid", rules[0].Pattern.String())
}

func TestReadLocalRuleListReportsMissingFileWithoutPanicking(t *testing.T) {
	rules := api.ReadLocalRuleList(filepath.Join(t.TempDir(), "does-not-exist"))

	assert.Empty(t, rules)
}

func TestReadLocalRuleListWithEmptyPathReturnsNothing(t *testing.T) {
	assert.Empty(t, api.ReadLocalRuleList(""))
}

func TestAppendRuleSkipsInvalidPattern(t *testing.T) {
	rules := api.AppendRule(nil, 7, "(unclosed")
	assert.Empty(t, rules)

	rules = api.AppendRule(rules, 7, "ok")
	require.Len(t, rules, 1)
	assert.Equal(t, 7, rules[0].ID)
	assert.Equal(t, "ok", rules[0].Pattern.String())
}

func TestNewHTTPClientAppliesTimeoutAndBaseURL(t *testing.T) {
	client := api.NewHTTPClient(&api.Config{APIHost: "https://panel.example.com"})
	assert.Equal(t, "https://panel.example.com", client.BaseURL)
	assert.Equal(t, 5*time.Second, client.GetClient().Timeout, "default timeout")

	configured := api.NewHTTPClient(&api.Config{APIHost: "https://panel.example.com", Timeout: 30})
	assert.Equal(t, 30*time.Second, configured.GetClient().Timeout)
}

func TestCheckResponseAcceptsSuccessfulResponses(t *testing.T) {
	response := &resty.Response{RawResponse: &http.Response{StatusCode: http.StatusOK}}
	assert.NoError(t, api.CheckResponse(response, "/path", "Panel", 1, nil))
}

func TestCheckResponseClassifiesHTTPFailures(t *testing.T) {
	for name, testCase := range map[string]struct {
		status   int
		expected api.ErrorKind
	}{
		"unauthorized":  {status: http.StatusUnauthorized, expected: api.ErrorAuthentication},
		"forbidden":     {status: http.StatusForbidden, expected: api.ErrorAuthentication},
		"not found":     {status: http.StatusNotFound, expected: api.ErrorNotFound},
		"rate limited":  {status: http.StatusTooManyRequests, expected: api.ErrorRateLimited},
		"bad request":   {status: http.StatusBadRequest, expected: api.ErrorInvalidPayload},
		"server errors": {status: http.StatusBadGateway, expected: api.ErrorServer},
	} {
		t.Run(name, func(t *testing.T) {
			response := &resty.Response{RawResponse: &http.Response{StatusCode: testCase.status}}

			err := api.CheckResponse(response, "/path", "Panel", 12, nil)

			var apiErr *api.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, testCase.expected, apiErr.Kind)
			assert.Equal(t, 12, apiErr.NodeID)
			assert.Equal(t, "/path", apiErr.Operation)
		})
	}
}

func TestCheckResponseClassifiesTransportErrors(t *testing.T) {
	err := api.CheckResponse(nil, "/path", "Panel", 12, context.DeadlineExceeded)

	var apiErr *api.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, api.ErrorTimeout, apiErr.Kind)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	err = api.CheckResponse(nil, "/path", "Panel", 12, stderrors.New("connection reset by peer"))
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, api.ErrorServer, apiErr.Kind)
}

func TestAPIErrorRedactsCredentialsFromTransportErrors(t *testing.T) {
	cause := stderrors.New(`Get "https://panel.example.com/api/v1/server/UniProxy/config?token=super-secret": dial tcp: i/o timeout`)

	err := api.ClassifyError("/config", "Xboard", 12, 0, cause)

	assert.NotContains(t, err.Error(), "super-secret")
	assert.Contains(t, err.Error(), "node 12")
	assert.Contains(t, err.Error(), "/config")
}
