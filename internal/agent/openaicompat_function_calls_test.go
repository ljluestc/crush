package agent

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestExtractLegacyFunctionCallCommands(t *testing.T) {
	t.Parallel()

	text := `prefix
<function_calls>pwd</function_calls>
middle
<function_calls>
ls -la
</function_calls>
suffix`

	commands := extractLegacyFunctionCallCommands(text)
	require.Equal(t, []string{"pwd", "ls -la"}, commands)
}

func TestStripLegacyFunctionCallMarkup(t *testing.T) {
	t.Parallel()

	text := `Let me inspect this.
<function_calls>pwd</function_calls>
Then continue.`

	require.Equal(t, "Let me inspect this.\n\nThen continue.", stripLegacyFunctionCallMarkup(text))
}

func TestEmitLegacyFunctionCallsAsBashToolCalls(t *testing.T) {
	t.Parallel()

	var emitted []fantasy.StreamPart
	yield := func(part fantasy.StreamPart) bool {
		emitted = append(emitted, part)
		return true
	}
	ok := emitLegacyFunctionCallsAsBashToolCalls(0, []string{"pwd"}, yield)
	require.True(t, ok)
	require.Len(t, emitted, 4)
	require.Equal(t, fantasy.StreamPartTypeToolInputStart, emitted[0].Type)
	require.Equal(t, fantasy.StreamPartTypeToolInputDelta, emitted[1].Type)
	require.Equal(t, fantasy.StreamPartTypeToolInputEnd, emitted[2].Type)
	require.Equal(t, fantasy.StreamPartTypeToolCall, emitted[3].Type)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(emitted[1].Delta), &payload))
	require.Equal(t, "pwd", payload["command"])
	require.Equal(t, legacyFunctionCallDescription, payload["description"])
}

