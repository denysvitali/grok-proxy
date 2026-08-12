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

func TestAnthropicRequestTranslatesBase64Image(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model: "claude-sonnet", MaxTokens: 100,
		Messages: []anthropic.Message{{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"what is this?"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc123"}}]`)}},
	}
	translated, err := AnthropicRequest(request, "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	var items []openai.InputItem
	if err := json.Unmarshal(translated.Input, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %#v", items)
	}
	var parts []openai.InputContent
	if err := json.Unmarshal(items[0].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Type != "input_text" || parts[1].Type != "input_image" {
		t.Fatalf("unexpected content parts: %#v", parts)
	}
	if parts[1].ImageURL != "data:image/png;base64,abc123" {
		t.Fatalf("unexpected image url: %q", parts[1].ImageURL)
	}
}

func TestAnthropicRequestTranslatesURLImage(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model: "claude-sonnet", MaxTokens: 100,
		Messages: []anthropic.Message{{Role: "user", Content: json.RawMessage(`[{"type":"image","source":{"type":"url","url":"https://example.test/a.png"}}]`)}},
	}
	translated, err := AnthropicRequest(request, "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	var items []openai.InputItem
	if err := json.Unmarshal(translated.Input, &items); err != nil {
		t.Fatal(err)
	}
	var parts []openai.InputContent
	if err := json.Unmarshal(items[0].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Type != "input_image" || parts[0].ImageURL != "https://example.test/a.png" {
		t.Fatalf("unexpected content parts: %#v", parts)
	}
}

func TestAnthropicRequestTranslatesImageToolResult(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model: "claude-sonnet", MaxTokens: 100,
		Messages: []anthropic.Message{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"Read","input":{"file_path":"shot.png"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc123"}}]}]`)},
		},
	}
	translated, err := AnthropicRequest(request, "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	var items []openai.InputItem
	if err := json.Unmarshal(translated.Input, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Type != "function_call" || items[1].Type != "function_call_output" || items[2].Type != "message" {
		t.Fatalf("unexpected translated input: %#v", items)
	}
	if items[1].Output != "" {
		t.Fatalf("expected empty text output, got %q", items[1].Output)
	}
	var parts []openai.InputContent
	if err := json.Unmarshal(items[2].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Type != "input_image" || parts[0].ImageURL != "data:image/png;base64,abc123" {
		t.Fatalf("unexpected image follow-up: %#v", parts)
	}
}

func TestAnthropicRequestOmitsToolChoiceWithoutTools(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model:      "claude-sonnet",
		MaxTokens:  100,
		ToolChoice: &anthropic.ToolChoice{Type: "auto"},
		Messages:   []anthropic.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	translated, err := AnthropicRequest(request, "grok-4.5")
	if err != nil {
		t.Fatal(err)
	}
	if len(translated.Tools) != 0 || len(translated.ToolChoice) != 0 {
		t.Fatalf("expected omitted tools/tool_choice, got tools=%s tool_choice=%s", translated.Tools, translated.ToolChoice)
	}
}

func TestAnthropicRequestRejectsIncompleteImage(t *testing.T) {
	request := anthropic.MessagesRequest{
		Model: "claude-sonnet", MaxTokens: 100,
		Messages: []anthropic.Message{{Role: "user", Content: json.RawMessage(`[{"type":"image","source":{"type":"base64"}}]`)}},
	}
	if _, err := AnthropicRequest(request, "grok-4.5"); err == nil {
		t.Fatal("expected incomplete image source to be rejected")
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
