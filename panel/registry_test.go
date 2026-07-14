package panel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupPanelAliases(t *testing.T) {
	for _, name := range []string{"Xboard", "xboard", "NewV2board", "V2BOARD"} {
		definition, err := LookupPanel(name)
		require.NoError(t, err)
		assert.Equal(t, "Xboard", definition.Name)
		assert.Equal(t, "NewV2board", definition.Adapter)
	}
}

func TestLookupPanelSuggestion(t *testing.T) {
	_, err := LookupPanel("Xbord")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Xboard")
}
