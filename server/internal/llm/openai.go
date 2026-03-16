package llm

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	openai "github.com/sashabaranov/go-openai"
)

type Client struct {
	client       *openai.Client
	systemPrompt string
	mu           sync.Mutex
	history      []openai.ChatCompletionMessage
}

func NewClient(apiKey, systemPrompt string) *Client {
	return &Client{
		client:       openai.NewClient(apiKey),
		systemPrompt: systemPrompt,
		history: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		},
	}
}

// Chat sends a user message and streams back the assistant response.
// onChunk is called with each text chunk, onSentence is called with each
// complete sentence (for TTS), and the full response is returned.
func (c *Client) Chat(ctx context.Context, userText string, onChunk func(string), onSentence func(string)) (string, error) {
	c.mu.Lock()
	c.history = append(c.history, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userText,
	})
	messages := make([]openai.ChatCompletionMessage, len(c.history))
	copy(messages, c.history)
	c.mu.Unlock()

	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    openai.GPT4oMini,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return "", fmt.Errorf("create chat stream: %w", err)
	}
	defer stream.Close()

	var fullResponse strings.Builder
	var sentenceBuf strings.Builder

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fullResponse.String(), fmt.Errorf("stream recv: %w", err)
		}

		chunk := resp.Choices[0].Delta.Content
		if chunk == "" {
			continue
		}

		fullResponse.WriteString(chunk)
		sentenceBuf.WriteString(chunk)

		if onChunk != nil {
			onChunk(chunk)
		}

		// Flush on sentence boundaries for TTS
		if onSentence != nil {
			text := sentenceBuf.String()
			if idx := findSentenceEnd(text); idx >= 0 {
				sentence := strings.TrimSpace(text[:idx+1])
				if sentence != "" {
					onSentence(sentence)
				}
				sentenceBuf.Reset()
				sentenceBuf.WriteString(text[idx+1:])
			}
		}
	}

	// Flush remaining text
	if onSentence != nil {
		remaining := strings.TrimSpace(sentenceBuf.String())
		if remaining != "" {
			onSentence(remaining)
		}
	}

	c.mu.Lock()
	c.history = append(c.history, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: fullResponse.String(),
	})
	// Keep history manageable (system + last 20 messages)
	if len(c.history) > 21 {
		kept := []openai.ChatCompletionMessage{c.history[0]}
		kept = append(kept, c.history[len(c.history)-20:]...)
		c.history = kept
	}
	c.mu.Unlock()

	log.Printf("[llm] response: %s", truncate(fullResponse.String(), 80))
	return fullResponse.String(), nil
}

func findSentenceEnd(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' || s[i] == '!' || s[i] == '?' {
			return i
		}
	}
	return -1
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: c.systemPrompt},
	}
}
