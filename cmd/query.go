package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/denysvitali/grok-proxy/internal/openai"
	"github.com/spf13/cobra"
)

func newModelsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available Grok models",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			var response json.RawMessage
			if err := runtime.grok.JSON(command.Context(), http.MethodGet, "/models", "", nil, &response); err != nil {
				return err
			}
			var formatted json.RawMessage
			if err := json.Unmarshal(response, &formatted); err != nil {
				return err
			}
			indented, err := json.MarshalIndent(formatted, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), string(indented))
			return nil
		},
	}
}

func newChatCommand() *cobra.Command {
	var model string
	var backend string
	var noStream bool
	command := &cobra.Command{
		Use:   "chat PROMPT",
		Short: "Send a prompt to Grok",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			encoded, path, err := chatRequest(backend, model, arguments[0], !noStream)
			if err != nil {
				return err
			}
			if noStream {
				var response json.RawMessage
				if err := runtime.grok.JSONBytes(command.Context(), http.MethodPost, path, model, encoded, &response); err != nil {
					return err
				}
				indented, err := json.MarshalIndent(response, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(command.OutOrStdout(), string(indented))
				return nil
			}

			response, err := runtime.grok.Do(command.Context(), http.MethodPost, path, model, encoded, "text/event-stream")
			if err != nil {
				return err
			}
			defer response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return grok.ReadError(response)
			}
			return printTextStream(command.OutOrStdout(), response.Body)
		},
	}
	command.Flags().StringVarP(&model, "model", "m", "grok-4.5", "model")
	command.Flags().StringVar(&backend, "backend", "responses", "responses or chat_completions")
	command.Flags().BoolVar(&noStream, "no-stream", false, "disable streaming")
	return command
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

func chatRequest(backend, model, prompt string, stream bool) ([]byte, string, error) {
	switch backend {
	case "responses":
		input, _ := json.Marshal(prompt)
		request := openai.ResponsesRequest{Model: model, Input: input, Stream: stream}
		encoded, err := json.Marshal(request)
		return encoded, "/responses", err
	case "chat_completions":
		request := chatCompletionsRequest{
			Model:    model,
			Messages: []chatMessage{{Role: "user", Content: prompt}},
			Stream:   stream,
		}
		encoded, err := json.Marshal(request)
		return encoded, "/chat/completions", err
	default:
		return nil, "", fmt.Errorf("unsupported backend %q", backend)
	}
}

type chatCompletionsChunk struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Delta chatDelta `json:"delta"`
}

type chatDelta struct {
	Content string `json:"content"`
}

func printTextStream(writer io.Writer, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var responseEvent openai.StreamEvent
		if json.Unmarshal([]byte(data), &responseEvent) == nil && responseEvent.Delta != "" {
			fmt.Fprint(writer, responseEvent.Delta)
			continue
		}
		var chatEvent chatCompletionsChunk
		if json.Unmarshal([]byte(data), &chatEvent) == nil && len(chatEvent.Choices) != 0 {
			fmt.Fprint(writer, chatEvent.Choices[0].Delta.Content)
		}
	}
	fmt.Fprintln(writer)
	return scanner.Err()
}
