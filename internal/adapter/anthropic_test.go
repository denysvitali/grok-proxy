package adapter

import (
	"encoding/json"
	"testing"

	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func TestAnthropicRequestTranslatesConversationAndTools(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model:     "claude-sonnet",
		System:    json.RawMessage(`"Be concise"`),
		MaxTokens: 1024,
		Messages: []anthropic.Message{
			{Role: "user", Content: json.RawMessage(`"Run a command"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"shell","input":{"command":"pwd"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"/tmp"}]`)},
		},
		Tools: []anthropic.Tool{{Name: "shell", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}
	translated, err := AnthropicRequest(request, "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	if translated.Model != "grok-4.5" || translated.Instructions != "Be concise" || translated.MaxOutputTokens != 1024 {
		t.Fatalf("unexpected translated request: %#v", translated)
	}
	var items []openai.InputItem
	if err := json.Unmarshal(translated.Input, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[1].Type != "function_call" || items[2].Type != "function_call_output" {
		t.Fatalf("unexpected translated input: %#v", items)
	}
}

func TestAnthropicRequestRejectsImages(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model: "claude-sonnet", MaxTokens: 100,
		Messages: []anthropic.Message{{Role: "user", Content: json.RawMessage(`[{"type":"image","source":{"type":"base64"}}]`)}},
	}
	if _, err := AnthropicRequest(request, "grok-4.5"); err == nil {
		t.Fatal("expected image input to be rejected")
	}
}

func TestAnthropicResponseTranslatesToolCall(t *testing.T) {
	response := openai.Response{
		ID:     "resp_1",
		Output: []openai.OutputItem{{Type: "function_call", CallID: "call_1", Name: "shell", Arguments: `{"command":"pwd"}`}},
		Usage:  openai.Usage{InputTokens: 10, OutputTokens: 5},
	}
	translated := AnthropicResponse(response, "claude-sonnet")
	if translated.StopReason != "tool_use" || len(translated.Content) != 1 || translated.Content[0].Name != "shell" {
		t.Fatalf("unexpected translated response: %#v", translated)
	}
}
