package adapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func AnthropicRequest(request anthropic.MessagesRequest, model string) (openai.ResponsesRequest, error) {
	if request.Model == "" {
		return openai.ResponsesRequest{}, errors.New("model is required")
	}
	if request.MaxTokens <= 0 {
		return openai.ResponsesRequest{}, errors.New("max_tokens must be greater than zero")
	}
	if len(request.StopSequences) != 0 {
		return openai.ResponsesRequest{}, errors.New("stop_sequences are not supported")
	}

	instructions, err := decodeText(request.System)
	if err != nil {
		return openai.ResponsesRequest{}, fmt.Errorf("system: %w", err)
	}

	items, err := anthropicInput(request.Messages)
	if err != nil {
		return openai.ResponsesRequest{}, err
	}
	input, err := json.Marshal(items)
	if err != nil {
		return openai.ResponsesRequest{}, err
	}

	var encodedTools json.RawMessage
	var toolChoice json.RawMessage
	parallelToolCalls := true
	if len(request.Tools) > 0 {
		tools := make([]openai.FunctionTool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			tools = append(tools, openai.FunctionTool{
				Type:        "function",
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
				Strict:      false,
			})
		}
		encoded, marshalErr := json.Marshal(tools)
		if marshalErr != nil {
			return openai.ResponsesRequest{}, marshalErr
		}
		encodedTools = encoded
		toolChoice = json.RawMessage(`"auto"`)
	}
	if request.ToolChoice != nil && (len(request.Tools) > 0 || request.ToolChoice.Type == "none") {
		parallelToolCalls = !request.ToolChoice.DisableParallelToolUse
		switch request.ToolChoice.Type {
		case "auto":
			toolChoice = json.RawMessage(`"auto"`)
		case "any":
			toolChoice = json.RawMessage(`"required"`)
		case "none":
			toolChoice = json.RawMessage(`"none"`)
		case "tool":
			encodedChoice, marshalErr := json.Marshal(openai.FunctionToolChoice{
				Type: "function",
				Name: request.ToolChoice.Name,
			})
			if marshalErr != nil {
				return openai.ResponsesRequest{}, marshalErr
			}
			toolChoice = encodedChoice
		default:
			return openai.ResponsesRequest{}, fmt.Errorf("unsupported tool_choice type %q", request.ToolChoice.Type)
		}
	}

	result := openai.ResponsesRequest{
		Model:             model,
		Instructions:      instructions,
		Input:             input,
		Tools:             encodedTools,
		ToolChoice:        toolChoice,
		ParallelToolCalls: parallelToolCalls,
		Store:             false,
		Stream:            request.Stream,
		MaxOutputTokens:   request.MaxTokens,
		Temperature:       request.Temperature,
		TopP:              request.TopP,
	}
	if request.Thinking != nil && request.Thinking.Type != "disabled" {
		result.Reasoning = &openai.Reasoning{Effort: "high", Summary: "auto"}
	}
	return result, nil
}

func AnthropicResponse(response openai.Response, requestedModel string) anthropic.MessagesResponse {
	content := make([]anthropic.ContentBlock, 0, len(response.Output))
	stopReason := "end_turn"
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					content = append(content, anthropic.ContentBlock{Type: "text", Text: part.Text})
				}
			}
		case "function_call":
			stopReason = "tool_use"
			arguments := json.RawMessage(item.Arguments)
			if !json.Valid(arguments) {
				arguments = json.RawMessage(`{}`)
			}
			content = append(content, anthropic.ContentBlock{
				Type:  "tool_use",
				ID:    item.CallID,
				Name:  item.Name,
				Input: arguments,
			})
		}
	}
	if response.Status == "incomplete" {
		stopReason = "max_tokens"
	}
	return anthropic.MessagesResponse{
		ID:         response.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      requestedModel,
		StopReason: stopReason,
		Usage: anthropic.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
		},
	}
}

func anthropicInput(messages []anthropic.Message) ([]openai.InputItem, error) {
	items := make([]openai.InputItem, 0, len(messages))
	for _, message := range messages {
		contentType := "input_text"
		if message.Role == "assistant" {
			contentType = "output_text"
		}
		var plainText string
		if err := json.Unmarshal(message.Content, &plainText); err == nil {
			content, _ := json.Marshal([]openai.InputContent{{Type: contentType, Text: plainText}})
			items = append(items, openai.InputItem{Type: "message", Role: message.Role, Content: content})
			continue
		}

		var blocks []anthropic.ContentBlock
		if err := json.Unmarshal(message.Content, &blocks); err != nil {
			return nil, errors.New("message content must be text or content blocks")
		}
		textParts := make([]openai.InputContent, 0, len(blocks))
		flushText := func() error {
			if len(textParts) == 0 {
				return nil
			}
			content, err := json.Marshal(textParts)
			if err != nil {
				return err
			}
			items = append(items, openai.InputItem{Type: "message", Role: message.Role, Content: content})
			textParts = textParts[:0]
			return nil
		}

		for _, block := range blocks {
			switch block.Type {
			case "text":
				textParts = append(textParts, openai.InputContent{Type: contentType, Text: block.Text})
			case "image":
				imageURL, err := imageURLFromSource(block.Source)
				if err != nil {
					return nil, err
				}
				textParts = append(textParts, openai.InputContent{Type: "input_image", ImageURL: imageURL})
			case "tool_use":
				if err := flushText(); err != nil {
					return nil, err
				}
				items = append(items, openai.InputItem{
					Type:      "function_call",
					CallID:    block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
				})
			case "tool_result":
				if err := flushText(); err != nil {
					return nil, err
				}
				output, images, err := decodeToolResult(block.Content)
				if err != nil {
					return nil, fmt.Errorf("tool result: %w", err)
				}
				items = append(items, openai.InputItem{
					Type:   "function_call_output",
					CallID: block.ToolUseID,
					Output: output,
				})
				if len(images) > 0 {
					content, marshalErr := json.Marshal(images)
					if marshalErr != nil {
						return nil, marshalErr
					}
					items = append(items, openai.InputItem{Type: "message", Role: "user", Content: content})
				}
			case "thinking", "redacted_thinking":
				// Thinking from earlier turns is provider-specific and is not replayed.
			default:
				return nil, fmt.Errorf("unsupported content block type %q", block.Type)
			}
		}
		if err := flushText(); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func decodeText(raw json.RawMessage) (string, error) {
	text, images, err := decodeContentBlocks(raw, false)
	if err != nil {
		return "", err
	}
	if len(images) > 0 {
		return "", errors.New("content contains a non-text block")
	}
	return text, nil
}

func decodeToolResult(raw json.RawMessage) (string, []openai.InputContent, error) {
	return decodeContentBlocks(raw, true)
}

func decodeContentBlocks(raw json.RawMessage, allowImages bool) (string, []openai.InputContent, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil, nil
	}
	var blocks []anthropic.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", nil, errors.New("content is not text")
	}
	var result strings.Builder
	var images []openai.InputContent
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if result.Len() != 0 {
				result.WriteByte('\n')
			}
			result.WriteString(block.Text)
		case "image":
			if !allowImages {
				return "", nil, errors.New("content contains a non-text block")
			}
			imageURL, err := imageURLFromSource(block.Source)
			if err != nil {
				return "", nil, err
			}
			images = append(images, openai.InputContent{Type: "input_image", ImageURL: imageURL})
		default:
			return "", nil, fmt.Errorf("unsupported content block type %q", block.Type)
		}
	}
	return result.String(), images, nil
}

func imageURLFromSource(source *anthropic.ImageSource) (string, error) {
	if source == nil {
		return "", errors.New("image block is missing source")
	}
	switch source.Type {
	case "base64":
		mediaType := strings.TrimSpace(source.MediaType)
		data := strings.TrimSpace(source.Data)
		if mediaType == "" {
			return "", errors.New("image base64 source requires media_type")
		}
		if data == "" {
			return "", errors.New("image base64 source requires data")
		}
		if strings.HasPrefix(data, "data:") {
			return data, nil
		}
		return "data:" + mediaType + ";base64," + data, nil
	case "url":
		url := strings.TrimSpace(source.URL)
		if url == "" {
			return "", errors.New("image url source requires url")
		}
		return url, nil
	default:
		return "", fmt.Errorf("unsupported image source type %q", source.Type)
	}
}
