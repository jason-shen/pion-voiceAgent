# Voice Agent

A real-time AI voice agent built with WebRTC. Speak naturally in your browser—the agent transcribes your speech, runs it through an LLM, and speaks back with low-latency TTS, all over a peer-to-peer audio connection. Signaling follows the [WHIP standard (RFC 9725)](https://www.rfc-editor.org/rfc/rfc9725.html) — a single HTTP POST exchanges the SDP offer/answer with no persistent signaling connection required.

## Features

- **Real-time voice conversation** — WebRTC audio streaming with sub-100ms round-trip
- **Configurable TTS** — [Cartesia Sonic](https://cartesia.ai/sonic) (default) or [Deepgram Aura](https://deepgram.com) for text-to-speech
- **Streaming STT** — [Deepgram](https://deepgram.com) real-time transcription with built-in VAD
- **LLM-powered** — [OpenAI GPT-4o-mini](https://platform.openai.com) with conversation history
- **Room-based** — Join named rooms; each room maintains its own conversation context
- **Open source** — Apache 2.0 licensed, Go + Next.js stack

## In Function Now

| Component | Status | Notes |
|-----------|--------|-------|
| WebRTC audio (Pion) | ✅ | Opus RTP bidirectional streaming |
| WHIP signaling | ✅ | Single HTTP POST, full ICE gathering |
| DataChannel events | ✅ | Transcript & response messages over DC |
| Room management | ✅ | Create/join rooms, goroutine per room |
| Deepgram STT | ✅ | Streaming WebSocket, Nova-2, endpointing |
| OpenAI LLM | ✅ | GPT-4o-mini, streaming, conversation history |
| Cartesia TTS | ✅ | Sonic-3, Katie voice, pcm_s16le |
| Deepgram TTS | ✅ | Aura Asteria, switchable via env |
| Next.js client | ✅ | Room join, live transcript, audio visualizer |

## TODO

- [ ] **Tool calling** — LLM function/tool execution (APIs, actions)
- [ ] **RAG** — Retrieval-augmented generation (documents, knowledge base)
- [ ] **Memory** — Persistent conversation memory across sessions

## Architecture

```
┌─────────────────────┐                    ┌─────────────────────────────────────┐
│   Next.js Client    │                    │           Go Server (Pion)           │
│                     │                    │                                       │
│  Mic → WebRTC ──────┼──── Opus RTP ──────┼──→ Opus Decode → Deepgram STT        │
│  Speaker ← WebRTC ←─┼──── Opus RTP ←─────┼──← Opus Encode ← TTS (Cartesia/DG)    │
│                     │                    │         │                            │
│  HTTP POST ─────────┼── WHIP (SDP) ──────┼──→ Peer created, answer returned       │
│  DataChannel ◄──────┼──── transcript ←───┼──← OpenAI GPT (streaming)              │
│                     │                    │                                       │
└─────────────────────┘                    └─────────────────────────────────────┘
```

**Signaling:** Client creates a DataChannel + SDP offer (with all ICE candidates gathered), POSTs it to `/whip/{room}`, and receives the SDP answer. No persistent signaling connection needed.

**Pipeline flow:** Browser captures mic → Opus over WebRTC → Server decodes to PCM → Deepgram STT → OpenAI LLM → Cartesia/Deepgram TTS → Opus over WebRTC → Browser plays audio. Transcript and response text are delivered back to the client via a WebRTC DataChannel.

## Prerequisites

**For Docker:** Docker and Docker Compose

**For local development:**
- **Go 1.22+**
- **Node.js 18+** and npm

**API keys (required):**
  - [Deepgram](https://deepgram.com) — required for STT
  - [OpenAI](https://platform.openai.com) — required for LLM
  - [Cartesia](https://cartesia.ai) — for TTS (default), or use Deepgram for TTS

## Quick Start

### Option A: Docker Compose (recommended)

**1. Configure the server**

```bash
cp server/.env.example server/.env
# Edit server/.env with your API keys
```

**2. Build and run**

```bash
docker compose up --build
```

**3. Open [http://localhost:3000](http://localhost:3000)** — enter a room name and click Connect.

> **Note:** The client connects to `http://localhost:8080/whip` by default. If you access the app via a different host (e.g. `http://192.168.1.x:3000`), rebuild the client with:
> ```bash
> docker compose build --build-arg NEXT_PUBLIC_WHIP_URL=http://YOUR_HOST:8080/whip client
> docker compose up
> ```

### Option B: Local development

**1. Clone and configure the server**

```bash
cd server
cp .env.example .env
# Edit .env with your API keys
```

**2. Start the server**

```bash
go run main.go
```

**3. Start the client**

```bash
cd client
cp .env.local.example .env.local
npm install
npm run dev
```

**4. Open [http://localhost:3000](http://localhost:3000)** — enter a room name and click Connect.

## Configuration

### Server (`server/.env`)

| Variable | Required | Default | Description |
|----------|----------|---------|--------------|
| `DEEPGRAM_API_KEY` | Yes | — | Deepgram API key (STT) |
| `OPENAI_API_KEY` | Yes | — | OpenAI API key (LLM) |
| `TTS_PROVIDER` | No | `cartesia` | `cartesia` or `deepgram` |
| `CARTESIA_API_KEY` | If Cartesia | — | [Cartesia](https://cartesia.ai) API key |
| `CARTESIA_VOICE_ID` | No | Katie | Voice ID (see [Cartesia docs](https://docs.cartesia.ai)) |
| `PORT` | No | `8080` | HTTP server port |
| `SYSTEM_PROMPT` | No | (helpful assistant) | System prompt for the agent |

### Client (`client/.env.local`)

| Variable | Required | Default | Description |
|----------|----------|---------|--------------|
| `NEXT_PUBLIC_WHIP_URL` | No | `http://localhost:8080/whip` | WHIP signaling endpoint |

## Signaling Protocol (WHIP — RFC 9725)

Signaling follows [RFC 9725](https://www.rfc-editor.org/rfc/rfc9725.html) (WebRTC-HTTP Ingestion Protocol).

### HTTP — SDP Exchange

| Step | Method | Path | Body | Response |
|------|--------|------|------|----------|
| 1 | `POST` | `/whip/{room}` | SDP offer (`application/sdp`) | `201 Created` with SDP answer, `Location` header, `ETag` header |
| 2 | `DELETE` | `/whip/{room}/{peerID}` | — | `200 OK` (session terminated) |
| — | `OPTIONS` | `/whip/*` | — | `204` with `Accept-Post: application/sdp` |

The client gathers all ICE candidates before sending the offer. The server gathers all ICE candidates before returning the answer. No trickle ICE.

### DataChannel — Event Messages

The client creates a DataChannel labelled `"events"` before generating the offer. Once the WebRTC connection is established, the server sends JSON text messages on it:

| Type | Payload | Description |
|------|---------|-------------|
| `transcript` | `{ text: string, final: boolean }` | User speech transcription (partial + final) |
| `response` | `{ text: string }` | LLM response chunks (streamed) |
| `error` | `{ message: string }` | Server-side errors |

### RFC 9725 Compliance

This implementation follows and aligns with [RFC 9725](https://www.rfc-editor.org/rfc/rfc9725.html):

- ✅ `POST` with `application/sdp` offer → `201 Created` with SDP answer (§4.2)
- ✅ `Location` header pointing to WHIP session URL (§4.2)
- ✅ `ETag` header identifying the ICE session (§4.3.1)
- ✅ `DELETE` on session URL for teardown (§4.2)
- ✅ `OPTIONS` with `Accept-Post: application/sdp` for CORS (§4.2)
- ✅ Full ICE gathering (no trickle ICE) on both client and server (§4.3.2)

**Bidirectional extensions:** WHIP was designed for unidirectional ingestion. This project extends it for bidirectional voice by using `sendrecv` (allowed per §4.2: client "MAY use sendrecv") and adding a DataChannel for server→client event delivery.

## Docker

| File | Purpose |
|------|---------|
| `server/Dockerfile` | Multi-stage Go build (pure Go, no CGO) |
| `client/Dockerfile` | Multi-stage Next.js build |
| `docker-compose.yml` | Orchestrates server + client |

The server reads env vars from `server/.env`. Create it from `server/.env.example` before running `docker compose up`.

## Project Structure

```
voiceagent/
├── docker-compose.yml
├── server/                 # Go backend
│   ├── Dockerfile
│   ├── main.go
│   └── internal/
│       ├── config/         # Env config
│       ├── signaling/      # WHIP HTTP signaling
│       ├── room/           # Room manager, per-room peers
│       ├── peer/           # Pion WebRTC peer
│       ├── pipeline/       # STT → LLM → TTS orchestration
│       ├── stt/            # Deepgram streaming STT
│       ├── llm/            # OpenAI chat completions
│       ├── tts/            # Cartesia + Deepgram TTS
│       └── audio/          # Opus encode/decode, PCM
│
├── client/                 # Next.js frontend
│   ├── Dockerfile
│   └── src/
│       ├── app/            # layout, page
│       ├── components/     # VoiceAgent, AudioVisualizer
│       ├── hooks/          # useVoiceAgent
│       └── lib/            # WHIP client
│
└── README.md
```

## Scaling

The room manager is in-memory (single process). For horizontal scaling, add a Redis-backed room manager with pub/sub for cross-instance signaling.

## Contributing

Contributions welcome. See [TODO](#todo) for planned features.

## License

Apache 2.0 — see [LICENSE](LICENSE).
