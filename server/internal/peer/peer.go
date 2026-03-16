package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/voiceagent/server/internal/audio"
)

type Peer struct {
	ID string
	pc *webrtc.PeerConnection
	ctx    context.Context
	cancel context.CancelFunc

	localTrack *webrtc.TrackLocalStaticRTP
	decoder    *audio.OpusDecoder
	encoder    *audio.OpusEncoder

	sendMu  sync.Mutex
	sendMsg func(interface{}) error

	OnAudioReceived func(pcm []int16)
	OnTrackReady    func()
	OnClose         func()

	// RTP state: protected by rtpMu
	rtpMu        sync.Mutex
	seqNum       uint16
	timestamp    uint32
	ssrc         uint32
	markerNext   bool
	lastSendTime time.Time

	closed bool
	mu     sync.Mutex
}

type candidateMsg struct {
	Type      string                   `json:"type"`
	Candidate *webrtc.ICECandidateInit `json:"candidate"`
}

func New(ctx context.Context, id string, sendMsg func(interface{}) error) (*Peer, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    1,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("register opus codec: %w", err)
	}

	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, fmt.Errorf("register interceptors: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  1,
		},
		"audio-agent",
		"voiceagent",
	)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create local track: %w", err)
	}

	if _, err := pc.AddTrack(track); err != nil {
		pc.Close()
		return nil, fmt.Errorf("add track: %w", err)
	}

	dec, err := audio.NewOpusDecoder()
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create opus decoder: %w", err)
	}

	enc, err := audio.NewOpusEncoder()
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}

	peerCtx, peerCancel := context.WithCancel(ctx)

	p := &Peer{
		ID:         id,
		pc:         pc,
		ctx:        peerCtx,
		cancel:     peerCancel,
		localTrack: track,
		decoder:    dec,
		encoder:    enc,
		sendMsg:    sendMsg,
		ssrc:       12345678,
		markerNext: true,
	}

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("[peer:%s] got remote track: %s", id, remoteTrack.Codec().MimeType)
		if p.OnTrackReady != nil {
			p.OnTrackReady()
		}
		go p.readTrack(remoteTrack)
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		msg := candidateMsg{Type: "candidate", Candidate: &init}
		p.sendMu.Lock()
		defer p.sendMu.Unlock()
		if err := p.sendMsg(msg); err != nil {
			log.Printf("[peer:%s] send ICE candidate error: %v", id, err)
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[peer:%s] connection state: %s", id, state.String())
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected ||
			state == webrtc.PeerConnectionStateClosed {
			p.Close()
		}
	})

	return p, nil
}

func (p *Peer) readTrack(track *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		n, _, err := track.Read(buf)
		if err != nil {
			log.Printf("[peer:%s] track read error: %v", p.ID, err)
			return
		}

		rtpPacket := &rtp.Packet{}
		if err := rtpPacket.Unmarshal(buf[:n]); err != nil {
			continue
		}

		pcm, err := p.decoder.Decode(rtpPacket.Payload)
		if err != nil {
			continue
		}

		if p.OnAudioReceived != nil {
			p.OnAudioReceived(pcm)
		}
	}
}

// MarkNewTalkspurt tells the peer to set the RTP marker bit on the next
// outgoing packet and to advance the timestamp to cover the wall-clock gap.
func (p *Peer) MarkNewTalkspurt() {
	p.rtpMu.Lock()
	defer p.rtpMu.Unlock()

	p.markerNext = true

	if !p.lastSendTime.IsZero() {
		gap := time.Since(p.lastSendTime)
		if gap > 20*time.Millisecond {
			// Advance timestamp to cover the silence gap so the jitter
			// buffer doesn't think the new packets are massively late.
			gapSamples := uint32(gap.Seconds() * 48000)
			p.timestamp += gapSamples
		}
	}
}

// SendAudio encodes PCM samples to Opus and writes an RTP packet to the local track.
func (p *Peer) SendAudio(pcm []int16) error {
	opusData, err := p.encoder.Encode(pcm)
	if err != nil {
		return err
	}

	p.rtpMu.Lock()
	p.seqNum++
	p.timestamp += uint32(len(pcm))
	seq := p.seqNum
	ts := p.timestamp
	marker := p.markerNext
	p.markerNext = false
	p.lastSendTime = time.Now()
	p.rtpMu.Unlock()

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           p.ssrc,
			Marker:         marker,
		},
		Payload: opusData,
	}

	raw, err := pkt.Marshal()
	if err != nil {
		return err
	}

	_, err = p.localTrack.Write(raw)
	return err
}

func (p *Peer) HandleOffer(sdp string) (string, error) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("set remote description: %w", err)
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("create answer: %w", err)
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("set local description: %w", err)
	}

	gatherDone := webrtc.GatheringCompletePromise(p.pc)
	select {
	case <-gatherDone:
	case <-time.After(5 * time.Second):
		log.Printf("[peer:%s] ICE gathering timed out, using partial candidates", p.ID)
	}

	return p.pc.LocalDescription().SDP, nil
}

func (p *Peer) AddICECandidate(raw json.RawMessage) error {
	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return err
	}
	return p.pc.AddICECandidate(candidate)
}

func (p *Peer) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	p.cancel()
	p.pc.Close()

	if p.OnClose != nil {
		p.OnClose()
	}
	log.Printf("[peer:%s] closed", p.ID)
}
