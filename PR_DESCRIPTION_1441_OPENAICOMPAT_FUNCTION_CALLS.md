# Fix `#1441`: OpenAI-compatible Claude outputs raw `<function_calls>` instead of invoking tools
## Summary
This PR adds a compatibility fallback for OpenAI-compatible providers when a model emits legacy XML-style tool calls like:
`<function_calls>...</function_calls>`.

Instead of leaving that content as plain assistant text, Crush now:
- detects those legacy blocks in the streamed assistant content,
- converts each block into a synthetic `bash` tool call during streaming,
- executes those tool calls through the normal tool pipeline, and
- removes raw `<function_calls>` markup from the final assistant text message.

This addresses issue `#1441`, where OpenAI-compatible Claude Haiku 4.5 could print raw function-call tags instead of triggering actual tool execution.

## Problem
For some OpenAI-compatible model/provider combinations, structured `tool_calls` are not returned in the OpenAI tool-call format.  
Instead, models may emit Anthropic-style XML-like text blocks (`<function_calls>...</function_calls>`) in plain assistant output.

Before this PR:
- Crush treated that output as normal text only,
- no tool execution happened for those blocks,
- users saw raw function-call markup in chat output.

## Root Cause
Crush's OpenAI-compatible path depended on structured tool-call stream events from the provider adapter.  
When the provider returned only text (with embedded XML-style function calls), there was no fallback conversion path.

## Changes
### 1) OpenAI-compatible stream fallback for legacy function-call markup
Added `internal/agent/openaicompat_function_calls.go`:
- `extractLegacyFunctionCallCommands`: parses `<function_calls>...</function_calls>` blocks.
- `openAICompatLegacyFunctionCallStreamExtra`: wraps existing `openaicompat.StreamExtraFunc`, accumulates streamed text content, and on step finish emits synthetic tool input/tool call stream events for each extracted command.
- Synthetic calls are emitted as `bash` tool calls with JSON input:
  - `description`: `"Execute command from legacy function_calls output"`
  - `command`: extracted block content.

### 2) Wire fallback into openai-compat provider initialization
Updated `internal/agent/coordinator.go`:
- `buildOpenaiCompatProvider` now injects:
  - `openaicompat.WithLanguageModelOptions(openai.WithLanguageModelStreamExtraFunc(openAICompatLegacyFunctionCallStreamExtra))`

This preserves existing OpenAI-compatible reasoning behavior by delegating first to `openaicompat.StreamExtraFunc`, then applying legacy function-call fallback logic.

### 3) Remove raw legacy function-call markup from assistant text
Updated `internal/agent/agent.go`:
- In step finalization (`OnStepFinish`), assistant text is cleaned with `stripLegacyFunctionCallMarkup` before finish metadata is added.
- Added helper `setMessageTextContent` to replace existing text content cleanly without disturbing other message parts.

## Tests
Added `internal/agent/openaicompat_function_calls_test.go`:
- `TestExtractLegacyFunctionCallCommands`
- `TestStripLegacyFunctionCallMarkup`
- `TestEmitLegacyFunctionCallsAsBashToolCalls`

Executed:
- `go test ./internal/agent -run 'Test(ExtractLegacyFunctionCallCommands|StripLegacyFunctionCallMarkup|EmitLegacyFunctionCallsAsBashToolCalls)$'`
- `go test ./internal/agent`

Both passed.

## Impact
- Fixes tool-call fallback for OpenAI-compatible providers that return XML-style function-call text.
- Reduces user-facing raw `<function_calls>` noise.
- Keeps existing structured tool-call behavior unchanged for providers/models that already return proper `tool_calls`.

## Risk and Mitigation
- **Risk**: false positives if normal assistant text intentionally contains `<function_calls>` tags.
  - **Mitigation**: conversion only triggers on well-formed paired blocks, and cleanup only removes matching legacy markup patterns.
- **Risk**: provider-specific behavior differences.
  - **Mitigation**: fallback is scoped to OpenAI-compatible provider stream handling and preserves existing openaicompat extra-stream logic.

