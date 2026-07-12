package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	"github.com/charmbracelet/crush/internal/agent/tools"
	openaiapi "github.com/charmbracelet/openai-go"
)

const legacyFunctionCallDescription = "Execute command from legacy function_calls output"

var (
	legacyFunctionCallsRegex       = regexp.MustCompile(`(?is)<function_calls>(.*?)</function_calls>`)
	orphanLegacyFunctionCallsRegex = regexp.MustCompile(`(?i)</?function_calls>`)
)

func extractLegacyFunctionCallCommands(text string) []string {
	matches := legacyFunctionCallsRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		command := strings.TrimSpace(match[1])
		if command == "" {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}

func stripLegacyFunctionCallMarkup(text string) string {
	cleaned := legacyFunctionCallsRegex.ReplaceAllString(text, "")
	cleaned = orphanLegacyFunctionCallsRegex.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func emitLegacyFunctionCallsAsBashToolCalls(choiceIndex int, commands []string, yield func(fantasy.StreamPart) bool) bool {
	for commandIndex, command := range commands {
		inputBytes, err := json.Marshal(map[string]string{
			"description": legacyFunctionCallDescription,
			"command":     command,
		})
		if err != nil {
			return yield(fantasy.StreamPart{
				Type:  fantasy.StreamPartTypeError,
				Error: fmt.Errorf("failed to marshal legacy function call command: %w", err),
			})
		}
		toolCallID := fmt.Sprintf("compat-function-call-%d-%d", choiceIndex, commandIndex)
		toolInput := string(inputBytes)
		if !yield(fantasy.StreamPart{
			Type:         fantasy.StreamPartTypeToolInputStart,
			ID:           toolCallID,
			ToolCallName: tools.BashToolName,
		}) {
			return false
		}
		if !yield(fantasy.StreamPart{
			Type:  fantasy.StreamPartTypeToolInputDelta,
			ID:    toolCallID,
			Delta: toolInput,
		}) {
			return false
		}
		if !yield(fantasy.StreamPart{
			Type: fantasy.StreamPartTypeToolInputEnd,
			ID:   toolCallID,
		}) {
			return false
		}
		if !yield(fantasy.StreamPart{
			Type:          fantasy.StreamPartTypeToolCall,
			ID:            toolCallID,
			ToolCallName:  tools.BashToolName,
			ToolCallInput: toolInput,
		}) {
			return false
		}
	}
	return true
}

func openAICompatLegacyFunctionCallStreamExtra(
	chunk openaiapi.ChatCompletionChunk,
	yield func(fantasy.StreamPart) bool,
	ctx map[string]any,
) (map[string]any, bool) {
	updatedCtx, shouldContinue := openaicompat.StreamExtraFunc(chunk, yield, ctx)
	if !shouldContinue {
		return updatedCtx, false
	}
	if updatedCtx == nil {
		updatedCtx = make(map[string]any)
	}

	for choiceIndex, choice := range chunk.Choices {
		bufferKey := fmt.Sprintf("legacy_function_calls_buffer_%d", choiceIndex)
		if choice.Delta.Content != "" {
			currentBuffer, _ := updatedCtx[bufferKey].(string)
			updatedCtx[bufferKey] = currentBuffer + choice.Delta.Content
		}

		if choice.FinishReason == "" {
			continue
		}
		emittedKey := fmt.Sprintf("legacy_function_calls_emitted_%d", choiceIndex)
		if emitted, _ := updatedCtx[emittedKey].(bool); emitted {
			continue
		}
		bufferText, _ := updatedCtx[bufferKey].(string)
		commands := extractLegacyFunctionCallCommands(bufferText)
		if len(commands) == 0 {
			continue
		}
		if !emitLegacyFunctionCallsAsBashToolCalls(choiceIndex, commands, yield) {
			return updatedCtx, false
		}
		updatedCtx[emittedKey] = true
	}

	return updatedCtx, true
}

