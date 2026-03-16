# Voice Agent

A real-time AI voice agent built with WebRTC. Speak naturally in your browser—the agent transcribes your speech, runs it through an LLM, and speaks back with low-latency TTS, all over a peer-to-peer audio connection.

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
| WebSocket signaling | ✅ | SDP offer/answer, ICE candidates |
| Room management | ✅ | Create/join rooms, goroutine per room |
| Deepgram STT | ✅ | Streaming WebSocket, Nova-2, endpointing |
| OpenAI LLM | ✅ | GPT-4o-mini, streaming, conversation history |
| Cartesia TTS | ✅ | Sonic-3, Katie voice, pcm_s16le |
| Deepgram TTS | ✅ | Aura Asteria, switchable via env |
| Next.js client | ✅ | Room join, live transcript, audio visualizer |
| ICE candidate buffering | ✅ | Handles out-of-order signaling |

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
│  WebSocket ◄───────►│◄──────────────────►│  OpenAI GPT (streaming)               │
│  (signaling)        │                    │         │                            │
└─────────────────────┘                    └─────────────────────────────────────┘
```

**Pipeline flow:** Browser captures mic → Opus over WebRTC → Server decodes to PCM → Deepgram STT → OpenAI LLM → Cartesia/Deepgram TTS → Opus over WebRTC → Browser plays audio.

## Prerequisites

**For Docker:** Docker and Docker Compose

**For local development:**
- **Go 1.22+** (CGO enabled for Opus)
- **Node.js 18+** and npm
- **libopus** — `brew install opus` (macOS) or `apt install libopus-dev` (Ubuntu/Debian)

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

> **Note:** The client connects to `ws://localhost:8080/ws` by default. If you access the app via a different host (e.g. `http://192.168.1.x:3000`), rebuild the client with:
> ```bash
> docker compose build --build-arg NEXT_PUBLIC_SIGNALING_URL=ws://YOUR_HOST:8080/ws client
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
| `PORT` | No | `8080` | HTTP/WebSocket port |
| `SYSTEM_PROMPT` | No | (helpful assistant) | System prompt for the agent |

### Client (`client/.env.local`)

| Variable | Required | Default | Description |
|----------|----------|---------|--------------|
| `NEXT_PUBLIC_SIGNALING_URL` | No | `ws://localhost:8080/ws` | WebSocket signaling URL |

## Signaling Protocol

WebSocket messages at `/ws`:

| Direction | Type | Payload |
|-----------|------|---------|
| Client → Server | `join` | `{ room: string }` |
| Server → Client | `joined` | `{ peer_id: string }` |
| Client → Server | `offer` | `{ sdp: string }` |
| Server → Client | `answer` | `{ sdp: string }` |
| Both | `candidate` | `{ candidate: RTCIceCandidateInit }` |
| Server → Client | `transcript` | `{ text: string, final: boolean }` |
| Server → Client | `response` | `{ text: string }` |

## Docker

| File | Purpose |
|------|---------|
| `server/Dockerfile` | Multi-stage Go build with libopus (CGO) |
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
│       ├── signaling/      # WebSocket SDP/ICE exchange
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
│       └── lib/            # SignalingClient
│
└── README.md
```

## Scaling

The room manager is in-memory (single process). For horizontal scaling, add a Redis-backed room manager with pub/sub for cross-instance signaling.

## Contributing

Contributions welcome. See [TODO](#todo) for planned features.

## License

Apache 2.0 — see [LICENSE](LICENSE).
