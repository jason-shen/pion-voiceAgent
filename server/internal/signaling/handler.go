package signaling

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/voiceagent/server/internal/room"
)

// NewWHIPHandler returns an HTTP handler implementing WHIP signaling per
// RFC 9725. It handles two URL patterns:
//
//	POST   /whip/{room}            – Ingest session setup (SDP offer/answer)
//	DELETE /whip/{room}/{peerID}   – Session teardown
//	OPTIONS (any)                  – CORS preflight
func NewWHIPHandler(rm *room.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse path segments: /whip/{room} or /whip/{room}/{peerID}
		trimmed := strings.TrimPrefix(r.URL.Path, "/whip/")
		parts := strings.SplitN(trimmed, "/", 2)
		roomName := parts[0]

		switch r.Method {
		case http.MethodOptions:
			// RFC 9725 §4.2: MUST support OPTIONS for CORS.
			w.Header().Set("Accept-Post", "application/sdp")
			w.WriteHeader(http.StatusNoContent)

		case http.MethodPost:
			handleWHIPPost(w, r, rm, roomName)

		case http.MethodDelete:
			if len(parts) < 2 || parts[1] == "" {
				http.Error(w, "session URL required: /whip/{room}/{peerID}", http.StatusBadRequest)
				return
			}
			handleWHIPDelete(w, rm, roomName, parts[1])

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handleWHIPPost implements RFC 9725 §4.2 Ingest Session Setup.
func handleWHIPPost(w http.ResponseWriter, r *http.Request, rm *room.Manager, roomName string) {
	if roomName == "" {
		http.Error(w, "room name required in path: /whip/{room}", http.StatusBadRequest)
		return
	}

	// RFC 9725 §4.2: MUST have content type application/sdp.
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/sdp") {
		http.Error(w, "Content-Type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}

	offerBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read offer", http.StatusBadRequest)
		return
	}
	offerSDP := string(offerBytes)
	if offerSDP == "" {
		http.Error(w, "empty SDP offer", http.StatusBadRequest)
		return
	}

	peerID := uuid.New().String()
	log.Printf("[whip] peer %s joining room %s", peerID, roomName)

	currentRoom := rm.GetOrCreate(roomName)
	p, err := currentRoom.AddPeer(peerID)
	if err != nil {
		log.Printf("[whip] add peer error: %v", err)
		http.Error(w, "failed to create peer", http.StatusInternalServerError)
		return
	}

	answerSDP, err := p.HandleOffer(offerSDP)
	if err != nil {
		log.Printf("[whip] offer error: %v", err)
		p.Close()
		http.Error(w, "failed to handle offer", http.StatusInternalServerError)
		return
	}

	log.Printf("[whip] peer %s connected to room %s", peerID, roomName)

	sessionURL := "/whip/" + roomName + "/" + peerID

	// RFC 9725 §4.2: 201 Created, application/sdp body, Location header.
	// RFC 9725 §4.3.1: ETag identifying the ICE session.
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", sessionURL)
	w.Header().Set("ETag", `"`+peerID+`"`)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(answerSDP))
}

// handleWHIPDelete implements RFC 9725 §4.2 session teardown.
func handleWHIPDelete(w http.ResponseWriter, rm *room.Manager, roomName, peerID string) {
	r := rm.Get(roomName)
	if r == nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	p := r.GetPeer(peerID)
	if p == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	log.Printf("[whip] DELETE session %s/%s", roomName, peerID)
	p.Close()
	w.WriteHeader(http.StatusOK)
}
