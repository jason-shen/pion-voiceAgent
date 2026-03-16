package stt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type TranscriptResult struct {
	Text    string
	IsFinal bool
}

type Client struct {
	apiKey   string
	ctx      context.Context
	cancel   context.CancelFunc
	ws       *websocket.Conn
	mu       sync.Mutex
	onResult func(TranscriptResult)
}

// deepgramResponse matches the Deepgram streaming API response shape.
type deepgramResponse struct {
	Type    string `json:"type"`
	Channel struct {
		Alternatives []struct {
			Transcript string  `json:"transcript"`
			Confidence float64 `json:"confidence"`
		} `json:"alternatives"`
	} `json:"channel"`
	IsFinal   bool `json:"is_final"`
	SpeechFinal bool `json:"speech_final"`
}

func NewClient(ctx context.Context, apiKey string, onResult func(TranscriptResult)) (*Client, error) {
	sttCtx, cancel := context.WithCancel(ctx)

	c := &Client{
		apiKey:   apiKey,
		ctx:      sttCtx,
		cancel:   cancel,
		onResult: onResult,
	}

	if err := c.connect(); err != nil {
		cancel()
		return nil, err
	}

	go c.readLoop()

	return c, nil
}

func (c *Client) connect() error {
	params := url.Values{}
	params.Set("encoding", "linear16")
	params.Set("sample_rate", "48000")
	params.Set("channels", "1")
	params.Set("model", "nova-2")
	params.Set("punctuate", "true")
	params.Set("interim_results", "true")
	params.Set("endpointing", "300")
	params.Set("vad_events", "true")

	u := fmt.Sprintf("wss://api.deepgram.com/v1/listen?%s", params.Encode())

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := map[string][]string{
		"Authorization": {fmt.Sprintf("Token %s", c.apiKey)},
	}

	ws, _, err := dialer.DialContext(c.ctx, u, header)
	if err != nil {
		return fmt.Errorf("deepgram connect: %w", err)
	}

	c.ws = ws
	log.Println("[stt] connected to Deepgram")
	return nil
}

func (c *Client) readLoop() {
	defer c.cancel()
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, data, err := c.ws.ReadMessage()
		if err != nil {
			if c.ctx.Err() == nil {
				log.Printf("[stt] read error: %v", err)
			}
			return
		}

		var resp deepgramResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}

		if resp.Type != "Results" || len(resp.Channel.Alternatives) == 0 {
			continue
		}

		text := resp.Channel.Alternatives[0].Transcript
		if text == "" {
			continue
		}

		c.onResult(TranscriptResult{
			Text:    text,
			IsFinal: resp.IsFinal || resp.SpeechFinal,
		})
	}
}

// SendAudio sends raw linear16 PCM bytes to Deepgram.
func (c *Client) SendAudio(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws == nil {
		return fmt.Errorf("not connected")
	}

	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Client) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws != nil {
		c.ws.WriteMessage(websocket.TextMessage, []byte(`{"type": "CloseStream"}`))
		c.ws.Close()
		c.ws = nil
	}
	log.Println("[stt] closed")
}
