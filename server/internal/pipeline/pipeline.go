package pipeline

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/voiceagent/server/internal/audio"
	"github.com/voiceagent/server/internal/config"
	"github.com/voiceagent/server/internal/llm"
	"github.com/voiceagent/server/internal/stt"
	"github.com/voiceagent/server/internal/tts"
)

type AudioSender interface {
	SendAudio(pcm []int16) error
	MarkNewTalkspurt()
}

type Pipeline struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config

	sttClient *stt.Client
	llmClient *llm.Client
	ttsClient tts.Client

	sendMsg func(interface{}) error
	sender  AudioSender
	senderMu sync.Mutex

	transcriptBuf strings.Builder
	transcriptMu  sync.Mutex

	processing sync.Mutex
}

type transcriptMsg struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Final bool   `json:"final"`
}

type responseMsg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func New(ctx context.Context, cfg *config.Config, sendMsg func(interface{}) error) (*Pipeline, error) {
	ttsClient, err := tts.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	pCtx, cancel := context.WithCancel(ctx)
	return &Pipeline{
		ctx:       pCtx,
		cancel:    cancel,
		cfg:       cfg,
		llmClient: llm.NewClient(cfg.OpenAIAPIKey, cfg.SystemPrompt),
		ttsClient: ttsClient,
		sendMsg:   sendMsg,
	}, nil
}

func (p *Pipeline) SetAudioSender(s AudioSender) {
	p.senderMu.Lock()
	defer p.senderMu.Unlock()
	p.sender = s
}

func (p *Pipeline) Start() error {
	sttClient, err := stt.NewClient(p.ctx, p.cfg.DeepgramAPIKey, p.onTranscript)
	if err != nil {
		return err
	}
	p.sttClient = sttClient
	log.Println("[pipeline] started")

	<-p.ctx.Done()
	return nil
}

func (p *Pipeline) HandleAudio(pcm []int16) {
	if p.sttClient == nil {
		return
	}
	data := audio.PCMToLinear16Bytes(pcm)
	if err := p.sttClient.SendAudio(data); err != nil {
		log.Printf("[pipeline] send audio to STT error: %v", err)
	}
}

func (p *Pipeline) onTranscript(result stt.TranscriptResult) {
	p.sendMsg(transcriptMsg{
		Type:  "transcript",
		Text:  result.Text,
		Final: result.IsFinal,
	})

	if !result.IsFinal {
		return
	}

	p.transcriptMu.Lock()
	if p.transcriptBuf.Len() > 0 {
		p.transcriptBuf.WriteString(" ")
	}
	p.transcriptBuf.WriteString(result.Text)
	fullText := p.transcriptBuf.String()
	p.transcriptBuf.Reset()
	p.transcriptMu.Unlock()

	log.Printf("[pipeline] final transcript: %s", fullText)
	go p.processResponse(fullText)
}

func (p *Pipeline) processResponse(userText string) {
	p.processing.Lock()
	defer p.processing.Unlock()

	if p.ctx.Err() != nil {
		return
	}

	// Channel decouples LLM sentence production from TTS playback so the
	// LLM stream isn't blocked while we're playing audio.
	sentences := make(chan string, 8)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.playSentences(sentences)
	}()

	_, err := p.llmClient.Chat(p.ctx, userText,
		func(chunk string) {
			p.sendMsg(responseMsg{
				Type: "response",
				Text: chunk,
			})
		},
		func(sentence string) {
			select {
			case sentences <- sentence:
			case <-p.ctx.Done():
			}
		},
	)
	close(sentences)

	if err != nil {
		log.Printf("[pipeline] LLM error: %v", err)
	}

	wg.Wait()
}

// playSentences reads sentences from the channel, synthesizes each via TTS,
// and streams the audio to the peer with proper RTP timing.
func (p *Pipeline) playSentences(sentences <-chan string) {
	for sentence := range sentences {
		if p.ctx.Err() != nil {
			return
		}
		p.synthesizeAndPlay(sentence)
	}
}

func (p *Pipeline) synthesizeAndPlay(text string) {
	if p.ctx.Err() != nil {
		return
	}

	pcmBytes, err := p.ttsClient.Synthesize(p.ctx, text)
	if err != nil {
		log.Printf("[pipeline] TTS error: %v", err)
		return
	}

	pcm := audio.Linear16BytesToPCM(pcmBytes)

	p.senderMu.Lock()
	sender := p.sender
	p.senderMu.Unlock()

	if sender == nil {
		return
	}

	// Signal new talkspurt so the receiver's jitter buffer resets timing
	// and the RTP marker bit is set on the first packet.
	sender.MarkNewTalkspurt()

	frameSize := audio.FrameSize
	frameDuration := 20 * time.Millisecond

	// Use wall-clock pacing: track when we started and when each frame
	// *should* be sent, rather than relying on ticker drift.
	start := time.Now()
	frameIndex := 0

	for i := 0; i < len(pcm); i += frameSize {
		if p.ctx.Err() != nil {
			return
		}

		// Sleep until this frame's scheduled send time
		targetTime := start.Add(time.Duration(frameIndex) * frameDuration)
		if wait := time.Until(targetTime); wait > 0 {
			time.Sleep(wait)
		}

		end := i + frameSize
		var frame []int16
		if end > len(pcm) {
			frame = make([]int16, frameSize)
			copy(frame, pcm[i:])
		} else {
			frame = pcm[i:end]
		}

		if err := sender.SendAudio(frame); err != nil {
			log.Printf("[pipeline] send audio error: %v", err)
			return
		}
		frameIndex++
	}
}

func (p *Pipeline) Stop() {
	p.cancel()
	if p.sttClient != nil {
		p.sttClient.Close()
	}
	log.Println("[pipeline] stopped")
}
