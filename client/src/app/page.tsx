import { VoiceAgent } from "../components/VoiceAgent";

export default function Home() {
  return (
    <main className="min-h-screen flex flex-col items-center justify-center px-4 py-12">
      <div className="text-center mb-10">
        <h1 className="text-3xl font-bold tracking-tight mb-2">
          AI Voice Agent
        </h1>
        <p className="text-zinc-500 text-sm max-w-md">
          Join a room and start talking. The AI agent will listen, understand,
          and respond in real time.
        </p>
      </div>
      <VoiceAgent />
    </main>
  );
}
