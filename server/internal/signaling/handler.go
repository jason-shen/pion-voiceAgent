package signaling

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/voiceagent/server/internal/peer"
	"github.com/voiceagent/server/internal/room"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type message struct {
	Type      string          `json:"type"`
	Room      string          `json:"room,omitempty"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

func NewHandler(rm *room.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[signaling] upgrade error: %v", err)
			return
		}
		defer ws.Close()

		peerID := uuid.New().String()
		var currentRoom *room.Room
		var currentPeer *peer.Peer

		log.Printf("[signaling] new connection: %s", peerID)

		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				log.Printf("[signaling] read error for %s: %v", peerID, err)
				break
			}

			var msg message
			if err := json.Unmarshal(raw, &msg); err != nil {
				log.Printf("[signaling] unmarshal error: %v", err)
				continue
			}

			switch msg.Type {
			case "join":
				if msg.Room == "" {
					sendError(ws, "room name required")
					continue
				}
				currentRoom = rm.GetOrCreate(msg.Room)
				p, err := currentRoom.AddPeer(peerID, ws)
				if err != nil {
					log.Printf("[signaling] add peer error: %v", err)
					sendError(ws, "failed to create peer")
					continue
				}
				currentPeer = p
				ws.WriteJSON(map[string]string{"type": "joined", "peer_id": peerID})
				log.Printf("[signaling] peer %s joined room %s", peerID, msg.Room)

			case "offer":
				if currentPeer == nil {
					sendError(ws, "must join a room first")
					continue
				}
				answerSDP, err := currentPeer.HandleOffer(msg.SDP)
				if err != nil {
					log.Printf("[signaling] offer error: %v", err)
					sendError(ws, "failed to handle offer")
					continue
				}
				ws.WriteJSON(map[string]string{"type": "answer", "sdp": answerSDP})

			case "candidate":
				if currentPeer == nil {
					continue
				}
				if err := currentPeer.AddICECandidate(msg.Candidate); err != nil {
					log.Printf("[signaling] ICE candidate error: %v", err)
				}

			default:
				log.Printf("[signaling] unknown message type: %s", msg.Type)
			}
		}

		if currentPeer != nil {
			currentPeer.Close()
		}
		log.Printf("[signaling] connection closed: %s", peerID)
	}
}

func sendError(ws *websocket.Conn, msg string) {
	ws.WriteJSON(map[string]string{"type": "error", "message": msg})
}
