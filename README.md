# Voice Agent

A real-time AI voice agent using WebRTC. Users connect to a room through a browser, speak naturally, and an AI agent listens (STT), thinks (LLM), and responds with speech (TTS) -- all streamed over WebRTC.

## Architecture

```
Browser (Next.js)  <-- WebRTC -->  Go Server (Pion)
                   <-- WebSocket (signaling) -->
```

**Audio pipeline on the server:**

1. Receive Opus audio from client via WebRTC
2. Decode Opus to PCM, stream to Deepgram STT (real-time WebSocket)
3. On final transcript, send to OpenAI GPT (streaming)
4. Stream LLM response sentences to TTS (Cartesia Sonic or Deepgram)
5. Encode TTS audio to Opus, send back via WebRTC

## Prerequisites

- **Go 1.22+** with CGO enabled (required for Opus codec via `libopus`)
- **Node.js 18+** and npm
- **libopus** development headers:
  - macOS: `brew install opus`
  - Ubuntu/Debian: `apt install libopus-dev`
- **Deepgram API key** -- [deepgram.com](https://deepgram.com) (required for STT)
- **OpenAI API key** -- [platform.openai.com](https://platform.openai.com)
- **Cartesia API key** (optional) -- [cartesia.ai](https://cartesia.ai) for TTS (default)

## Setup

### Server

```bash
cd server
cp .env.example .env
# Edit .env with your API keys

go run main.go
```

The server starts on port 8080 by default.

### Client

```bash
cd client
cp .env.local.example .env.local
# Edit if the server runs on a different host/port

npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000), enter a room name, and click Connect.

## Environment Variables

### Server (`server/.env`)

| Variable | Required | Default | Description |
|---|---|---|---|
| `DEEPGRAM_API_KEY` | Yes | -- | Deepgram API key (required for STT) |
| `OPENAI_API_KEY` | Yes | -- | OpenAI API key for LLM |
| `TTS_PROVIDER` | No | `cartesia` | TTS provider: `cartesia` or `deepgram` |
| `CARTESIA_API_KEY` | If cartesia | -- | Cartesia API key for TTS ([cartesia.ai](https://cartesia.ai)) |
| `CARTESIA_VOICE_ID` | No | Katie | Cartesia voice ID |
| `PORT` | No | `8080` | HTTP/WebSocket server port |
| `SYSTEM_PROMPT` | No | (helpful assistant) | System prompt for the AI agent |

### Client (`client/.env.local`)

| Variable | Required | Default | Description |
|---|---|---|---|
| `NEXT_PUBLIC_SIGNALING_URL` | No | `ws://localhost:8080/ws` | WebSocket signaling endpoint |

## Signaling Protocol

All signaling happens over a single WebSocket connection at `/ws`:

| Direction | Type | Payload |
|---|---|---|
| Client -> Server | `join` | `{ room: string }` |
| Server -> Client | `joined` | `{ peer_id: string }` |
| Client -> Server | `offer` | `{ sdp: string }` |
| Server -> Client | `answer` | `{ sdp: string }` |
| Bidirectional | `candidate` | `{ candidate: RTCIceCandidateInit }` |
| Server -> Client | `transcript` | `{ text: string, final: boolean }` |
| Server -> Client | `response` | `{ text: string }` |

## Project Structure

```
server/
  main.go                      Entry point
  internal/
    config/config.go           Environment-based config
    signaling/handler.go       WebSocket signaling
    room/manager.go            Room lifecycle management
    room/room.go               Per-room peer coordination
    peer/peer.go               Pion WebRTC peer wrapper
    pipeline/pipeline.go       STT -> LLM -> TTS orchestration
    stt/deepgram.go            Deepgram streaming STT
    llm/openai.go              OpenAI chat completions
    tts/deepgram.go            Deepgram TTS
    audio/opus.go              Opus encode/decode
    audio/rtp.go               PCM byte conversion

client/
  src/
    app/page.tsx               Main page
    components/VoiceAgent.tsx   Voice agent UI
    components/AudioVisualizer.tsx  Audio level bars
    hooks/useVoiceAgent.ts     WebRTC + signaling hook
    lib/signaling.ts           WebSocket signaling client
```

## Multi-Instance

The room manager currently runs in-memory (single process). To scale horizontally, replace the in-memory room manager with a Redis-backed implementation using pub/sub for cross-instance signaling. The `room.Manager` interface is designed for this swap.
