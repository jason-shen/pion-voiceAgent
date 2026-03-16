package room

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/voiceagent/server/internal/config"
	"github.com/voiceagent/server/internal/peer"
	"github.com/voiceagent/server/internal/pipeline"
)

type Room struct {
	ID     string
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	peers map[string]*peer.Peer
}

func NewRoom(id string, cfg *config.Config) *Room {
	ctx, cancel := context.WithCancel(context.Background())
	return &Room{
		ID:     id,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		peers:  make(map[string]*peer.Peer),
	}
}

// AddPeer creates a new Pion peer for the given WebSocket connection,
// wires up the audio pipeline, and starts processing.
func (r *Room) AddPeer(peerID string, ws *websocket.Conn) (*peer.Peer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var wsMu sync.Mutex
	sendMsg := func(msg interface{}) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return ws.WriteJSON(msg)
	}

	p, err := peer.New(r.ctx, peerID, sendMsg)
	if err != nil {
		return nil, err
	}

	pl, err := pipeline.New(r.ctx, r.cfg, sendMsg)
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
		r.removePeer(peerID)
	}

	r.peers[peerID] = p

	go func() {
		if err := pl.Start(); err != nil {
			log.Printf("[room:%s] pipeline error for peer %s: %v", r.ID, peerID, err)
		}
	}()

	return p, nil
}

func (r *Room) removePeer(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, peerID)
	log.Printf("[room:%s] peer %s removed, %d peers remaining", r.ID, peerID, len(r.peers))
}

func (r *Room) PeerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.peers)
}

func (r *Room) Close() {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.peers {
		p.Close()
	}
	r.peers = make(map[string]*peer.Peer)
	log.Printf("[room:%s] closed", r.ID)
}
