package session

import (
	"context"
	"log"
	"sync"

	"github.com/voiceagent/server/internal/config"
	"github.com/voiceagent/server/internal/peer"
	"github.com/voiceagent/server/internal/pipeline"
)

type Session struct {
	ID     string
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	peers map[string]*peer.Peer
}

func NewSession(id string, cfg *config.Config) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		ID:     id,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		peers:  make(map[string]*peer.Peer),
	}
}

// AddPeer creates a new Pion peer, wires up the audio pipeline, and starts
// processing. Event messages (transcript, response) are delivered via the
// peer's DataChannel.
func (s *Session) AddPeer(peerID string) (*peer.Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := peer.New(s.ctx, peerID)
	if err != nil {
		return nil, err
	}

	sendMsg := func(msg interface{}) error {
		return p.SendEvent(msg)
	}

	pl, err := pipeline.New(s.ctx, s.cfg, sendMsg)
	if err != nil {
		p.Close()
		return nil, err
	}
	p.OnAudioReceived = pl.HandleAudio
	p.OnTrackReady = func() {
		pl.SetAudioSender(p)
	}

	p.OnClose = func() {
		pl.Stop()
		s.removePeer(peerID)
	}

	s.peers[peerID] = p

	go func() {
		if err := pl.Start(); err != nil {
			log.Printf("[session:%s] pipeline error for peer %s: %v", s.ID, peerID, err)
		}
	}()

	return p, nil
}

func (s *Session) GetPeer(peerID string) *peer.Peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peers[peerID]
}

func (s *Session) removePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, peerID)
	log.Printf("[session:%s] peer %s removed, %d peers remaining", s.ID, peerID, len(s.peers))
}

func (s *Session) PeerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.peers)
}

func (s *Session) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.peers {
		p.Close()
	}
	s.peers = make(map[string]*peer.Peer)
	log.Printf("[session:%s] closed", s.ID)
}
