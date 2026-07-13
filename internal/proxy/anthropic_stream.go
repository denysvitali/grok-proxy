package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func writeAnthropicStream(w http.ResponseWriter, reader io.Reader, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	messageID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	writeSSE(w, "message_start", anthropic.StreamMessageStart{
		Type: "message_start",
		Message: anthropic.MessagesResponse{
			ID:      messageID,
			Type:    "message",
			Role:    "assistant",
			Content: []anthropic.ContentBlock{},
			Model:   model,
		},
	})
	flush(flusher)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	blockIndexes := make([]int, 0, 8)
	blockTypes := make([]string, 0, 8)
	nextBlockIndex := 0
	stopReason := "end_turn"
	var usage openai.Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event openai.StreamEvent
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		switch event.Type {
		case "response.output_item.added":
			if event.Item == nil {
				continue
			}
			for len(blockIndexes) <= event.OutputIndex {
				blockIndexes = append(blockIndexes, -1)
				blockTypes = append(blockTypes, "")
			}
			switch event.Item.Type {
			case "message":
				blockIndexes[event.OutputIndex] = nextBlockIndex
				blockTypes[event.OutputIndex] = "text"
				writeSSE(w, "content_block_start", anthropic.StreamContentStart{
					Type:         "content_block_start",
					Index:        nextBlockIndex,
					ContentBlock: anthropic.ContentBlock{Type: "text"},
				})
				nextBlockIndex++
			case "function_call":
				blockIndexes[event.OutputIndex] = nextBlockIndex
				blockTypes[event.OutputIndex] = "tool_use"
				stopReason = "tool_use"
				writeSSE(w, "content_block_start", anthropic.StreamContentStart{
					Type:  "content_block_start",
					Index: nextBlockIndex,
					ContentBlock: anthropic.ContentBlock{
						Type:  "tool_use",
						ID:    event.Item.CallID,
						Name:  event.Item.Name,
						Input: json.RawMessage(`{}`),
					},
				})
				nextBlockIndex++
			}
		case "response.output_text.delta":
			if index, ok := translatedIndex(blockIndexes, event.OutputIndex); ok {
				writeSSE(w, "content_block_delta", anthropic.StreamContentDelta{
					Type:  "content_block_delta",
					Index: index,
					Delta: anthropic.StreamDelta{Type: "text_delta", Text: event.Delta},
				})
			}
		case "response.function_call_arguments.delta":
			if index, ok := translatedIndex(blockIndexes, event.OutputIndex); ok {
				writeSSE(w, "content_block_delta", anthropic.StreamContentDelta{
					Type:  "content_block_delta",
					Index: index,
					Delta: anthropic.StreamDelta{Type: "input_json_delta", PartialJSON: event.Delta},
				})
			}
		case "response.output_item.done":
			if index, ok := translatedIndex(blockIndexes, event.OutputIndex); ok {
				writeSSE(w, "content_block_stop", anthropic.StreamContentStop{Type: "content_block_stop", Index: index})
			}
		case "response.completed":
			if event.Response != nil {
				usage = event.Response.Usage
				if event.Response.Status == "incomplete" {
					stopReason = "max_tokens"
				}
			}
		case "response.failed", "error":
			writeSSE(w, "error", anthropic.ErrorResponse{
				Type:  "error",
				Error: anthropic.ErrorBody{Type: "api_error", Message: "upstream stream failed"},
			})
			flush(flusher)
			return
		}
		flush(flusher)
	}

	writeSSE(w, "message_delta", anthropic.StreamMessageDelta{
		Type:  "message_delta",
		Delta: anthropic.MessageDeltaBody{StopReason: stopReason},
		Usage: anthropic.Usage{OutputTokens: usage.OutputTokens},
	})
	writeSSE(w, "message_stop", anthropic.StreamMessageStop{Type: "message_stop"})
	flush(flusher)
}

func translatedIndex(indexes []int, outputIndex int) (int, bool) {
	if outputIndex < 0 || outputIndex >= len(indexes) || indexes[outputIndex] < 0 {
		return 0, false
	}
	return indexes[outputIndex], true
}

func writeSSE[T any](writer io.Writer, event string, value T) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, encoded)
}

func flush(flusher http.Flusher) {
	if flusher != nil {
		flusher.Flush()
	}
}
