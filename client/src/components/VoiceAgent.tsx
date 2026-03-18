"use client";

import { useState } from "react";
import {
  useVoiceAgent,
  type ConnectionStatus,
  type TranscriptEntry,
} from "../hooks/useVoiceAgent";
import { AudioVisualizer } from "./AudioVisualizer";

function StatusBadge({ status }: { status: ConnectionStatus }) {
  const config: Record<ConnectionStatus, { color: string; label: string }> = {
    idle: { color: "bg-zinc-600", label: "Ready" },
    connecting: { color: "bg-yellow-500 animate-pulse", label: "Connecting..." },
    connected: { color: "bg-green-500", label: "Connected" },
    error: { color: "bg-red-500", label: "Error" },
    disconnected: { color: "bg-zinc-600", label: "Disconnected" },
  };

  const { color, label } = config[status];

  return (
    <div className="flex items-center gap-2 text-sm">
      <span className={`w-2.5 h-2.5 rounded-full ${color}`} />
      <span className="text-zinc-400">{label}</span>
    </div>
  );
}

function TranscriptPanel({ entries }: { entries: TranscriptEntry[] }) {
  return (
    <div className="flex flex-col gap-3 overflow-y-auto max-h-[400px] no-scrollbar">
      {entries.length === 0 && (
        <p className="text-zinc-600 text-sm text-center py-8">
          Conversation will appear here...
        </p>
      )}
      {entries.map((entry, i) => (
        <div
          key={i}
          className={`flex ${entry.role === "user" ? "justify-end" : "justify-start"}`}
        >
          <div
            className={`max-w-[80%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
              entry.role === "user"
                ? "bg-indigo-600/20 text-indigo-100 border border-indigo-500/20"
                : "bg-zinc-800/80 text-zinc-200 border border-zinc-700/50"
            } ${entry.partial ? "opacity-70" : ""}`}
          >
            <span className="text-[10px] uppercase tracking-wider text-zinc-500 block mb-1">
              {entry.role === "user" ? "You" : "Agent"}
            </span>
            {entry.text}
          </div>
        </div>
      ))}
    </div>
  );
}

export function VoiceAgent() {
  const [room, setRoom] = useState("");
  const {
    status,
    transcript,
    audioLevel,
    isMuted,
    connect,
    disconnect,
    toggleMute,
  } = useVoiceAgent();

  const isActive = status === "connected";
  const canConnect = status === "idle" || status === "disconnected" || status === "error";

  const handleConnect = () => {
    if (!room.trim()) return;
    connect(room.trim());
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && canConnect) {
      handleConnect();
    }
  };

  return (
    <div className="w-full max-w-lg mx-auto">
      <div className="rounded-2xl border border-[var(--card-border)] bg-[var(--card)] overflow-hidden shadow-2xl shadow-indigo-950/20">
        {/* Header */}
        <div className="px-6 py-5 border-b border-[var(--card-border)] flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">Voice Agent</h2>
            <StatusBadge status={status} />
          </div>
          {isActive && (
            <div className="flex gap-2">
              <button
                onClick={toggleMute}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                  isMuted
                    ? "bg-red-500/20 text-red-400 hover:bg-red-500/30"
                    : "bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
                }`}
              >
                {isMuted ? "Unmute" : "Mute"}
              </button>
              <button
                onClick={disconnect}
                className="px-3 py-1.5 rounded-lg text-xs font-medium bg-red-600/20 text-red-400 hover:bg-red-600/30 transition-colors"
              >
                Disconnect
              </button>
            </div>
          )}
        </div>

        {/* Visualizer */}
        <div className="px-6 py-6 border-b border-[var(--card-border)] flex flex-col items-center gap-4">
          <div
            className={`w-24 h-24 rounded-full flex items-center justify-center transition-all duration-300 ${
              isActive
                ? "bg-indigo-600/20 border-2 border-indigo-500/40"
                : "bg-zinc-800/50 border-2 border-zinc-700/30"
            }`}
          >
            {isActive && (
              <div
                className="absolute w-24 h-24 rounded-full border-2 border-indigo-400/30 pulse-ring"
              />
            )}
            <svg
              className={`w-8 h-8 ${isActive ? "text-indigo-400" : "text-zinc-600"}`}
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M12 18.75a6 6 0 006-6v-1.5m-6 7.5a6 6 0 01-6-6v-1.5m6 7.5v3.75m-3.75 0h7.5M12 15.75a3 3 0 01-3-3V4.5a3 3 0 116 0v8.25a3 3 0 01-3 3z"
              />
            </svg>
          </div>
          <AudioVisualizer level={audioLevel} active={isActive && !isMuted} />
        </div>

        {/* Room Input / Connect */}
        {canConnect && (
          <div className="px-6 py-5 border-b border-[var(--card-border)]">
            <div className="flex gap-3">
              <input
                type="text"
                value={room}
                onChange={(e) => setRoom(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Enter room name..."
                className="flex-1 bg-zinc-900/60 border border-zinc-700/50 rounded-xl px-4 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:ring-2 focus:ring-indigo-500/40 focus:border-indigo-500/40 transition-all"
              />
              <button
                onClick={handleConnect}
                disabled={!room.trim()}
                className="px-5 py-2.5 rounded-xl text-sm font-medium bg-indigo-600 text-white hover:bg-indigo-500 disabled:opacity-40 disabled:cursor-not-allowed transition-all"
              >
                Connect
              </button>
            </div>
          </div>
        )}

        {/* Transcript */}
        <div className="px-6 py-5 min-h-[200px]">
          <TranscriptPanel entries={transcript} />
        </div>
      </div>
    </div>
  );
}
