"use client";

import { useCallback, useRef, useState } from "react";
import { whipOffer, whipDelete } from "../lib/whipClient";

export type ConnectionStatus =
  | "idle"
  | "connecting"
  | "connected"
  | "error"
  | "disconnected";

export interface TranscriptEntry {
  role: "user" | "assistant";
  text: string;
  partial?: boolean;
}

export interface UseVoiceAgentReturn {
  status: ConnectionStatus;
  transcript: TranscriptEntry[];
  audioLevel: number;
  isMuted: boolean;
  connect: (room: string) => Promise<void>;
  disconnect: () => void;
  toggleMute: () => void;
}

type DataChannelMessage =
  | { type: "transcript"; text: string; final: boolean }
  | { type: "response"; text: string }
  | { type: "error"; message: string };

export function useVoiceAgent(): UseVoiceAgentReturn {
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [transcript, setTranscript] = useState<TranscriptEntry[]>([]);
  const [audioLevel, setAudioLevel] = useState(0);
  const [isMuted, setIsMuted] = useState(false);

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const sessionURLRef = useRef<string>("");
  const streamRef = useRef<MediaStream | null>(null);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const animFrameRef = useRef<number>(0);
  const assistantBufRef = useRef("");

  const cleanupAudioLevel = useCallback(() => {
    if (animFrameRef.current) {
      cancelAnimationFrame(animFrameRef.current);
      animFrameRef.current = 0;
    }
    setAudioLevel(0);
  }, []);

  const startAudioLevelMonitoring = useCallback((stream: MediaStream) => {
    const audioCtx = new AudioContext();
    audioCtxRef.current = audioCtx;
    const source = audioCtx.createMediaStreamSource(stream);
    const analyser = audioCtx.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);
    analyserRef.current = analyser;

    const dataArray = new Uint8Array(analyser.frequencyBinCount);

    const tick = () => {
      analyser.getByteFrequencyData(dataArray);
      let sum = 0;
      for (let i = 0; i < dataArray.length; i++) {
        sum += dataArray[i];
      }
      const avg = sum / dataArray.length / 255;
      setAudioLevel(avg);
      animFrameRef.current = requestAnimationFrame(tick);
    };
    tick();
  }, []);

  const handleDataChannelMessage = useCallback(
    (msg: DataChannelMessage) => {
      switch (msg.type) {
        case "transcript": {
          if (msg.final) {
            // Finalize assistant buffer first so the thread stays in order:
            // user → agent → user → agent
            const pendingAssistant = assistantBufRef.current;
            assistantBufRef.current = "";

            setTranscript((prev) => {
              let updated = prev.filter(
                (e) =>
                  !(e.role === "user" && e.partial) &&
                  !(e.role === "assistant" && e.partial)
              );
              if (pendingAssistant) {
                updated = [
                  ...updated,
                  { role: "assistant" as const, text: pendingAssistant },
                ];
              }
              return [...updated, { role: "user", text: msg.text }];
            });
          } else {
            setTranscript((prev) => {
              const updated = prev.filter(
                (e) => !(e.role === "user" && e.partial)
              );
              return [
                ...updated,
                { role: "user", text: msg.text, partial: true },
              ];
            });
          }
          break;
        }
        case "response": {
          assistantBufRef.current += msg.text;
          const currentText = assistantBufRef.current;
          setTranscript((prev) => {
            const updated = prev.filter(
              (e) => !(e.role === "assistant" && e.partial)
            );
            return [
              ...updated,
              { role: "assistant", text: currentText, partial: true },
            ];
          });
          break;
        }
        case "error": {
          console.error("[voiceagent] server error:", msg.message);
          break;
        }
      }
    },
    []
  );

  const connect = useCallback(
    async (room: string) => {
      try {
        setStatus("connecting");
        setTranscript([]);
        assistantBufRef.current = "";

        const pc = new RTCPeerConnection({
          iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
        });
        pcRef.current = pc;

        // Create a DataChannel for receiving events (transcript, response)
        // from the server. Must be created before the offer so it is
        // included in the SDP.
        const dc = pc.createDataChannel("events");
        dc.onmessage = (e) => {
          try {
            const msg = JSON.parse(e.data) as DataChannelMessage;
            handleDataChannelMessage(msg);
          } catch {
            console.error("[voiceagent] failed to parse DC message", e.data);
          }
        };

        // Store the remote audio element in a ref so it doesn't get GC'd.
        // Route through an AudioContext to guarantee playback starts even
        // before user gesture (we already have gesture from the Connect click).
        pc.ontrack = (e) => {
          const remoteStream =
            e.streams[0] || new MediaStream([e.track]);

          const audioEl = new Audio();
          audioEl.srcObject = remoteStream;
          audioEl.autoplay = true;
          remoteAudioRef.current = audioEl;

          // Force play (handles browsers that gate autoplay)
          audioEl.play().catch(() => {
            // Will retry on next user interaction
          });
        };

        pc.onconnectionstatechange = () => {
          const state = pc.connectionState;
          if (state === "connected") {
            setStatus("connected");
          } else if (state === "failed" || state === "closed") {
            setStatus("disconnected");
            cleanupAudioLevel();
          }
        };

        const stream = await navigator.mediaDevices.getUserMedia({
          audio: {
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
          },
        });
        streamRef.current = stream;

        stream.getTracks().forEach((track) => pc.addTrack(track, stream));
        startAudioLevelMonitoring(stream);

        // Create offer and gather all ICE candidates before sending.
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        // Wait for ICE gathering to complete so the offer contains all candidates.
        await new Promise<void>((resolve) => {
          if (pc.iceGatheringState === "complete") {
            resolve();
            return;
          }
          const onGatherChange = () => {
            if (pc.iceGatheringState === "complete") {
              pc.removeEventListener(
                "icegatheringstatechange",
                onGatherChange
              );
              resolve();
            }
          };
          pc.addEventListener("icegatheringstatechange", onGatherChange);
        });

        // WHIP exchange: POST offer SDP, receive answer SDP + session URL.
        const { answerSDP, sessionURL } = await whipOffer(
          room,
          pc.localDescription!.sdp
        );
        sessionURLRef.current = sessionURL;

        await pc.setRemoteDescription(
          new RTCSessionDescription({ type: "answer", sdp: answerSDP })
        );
      } catch (err) {
        console.error("[voiceagent] connect error:", err);
        setStatus("error");
      }
    },
    [handleDataChannelMessage, startAudioLevelMonitoring, cleanupAudioLevel]
  );

  const disconnect = useCallback(() => {
    cleanupAudioLevel();
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;

    if (remoteAudioRef.current) {
      remoteAudioRef.current.pause();
      remoteAudioRef.current.srcObject = null;
      remoteAudioRef.current = null;
    }
    audioCtxRef.current?.close();
    audioCtxRef.current = null;

    // RFC 9725 §4.2: DELETE the WHIP session to free server resources.
    whipDelete(sessionURLRef.current);
    sessionURLRef.current = "";

    pcRef.current?.close();
    pcRef.current = null;
    setStatus("idle");
    assistantBufRef.current = "";
  }, [cleanupAudioLevel]);

  const toggleMute = useCallback(() => {
    const stream = streamRef.current;
    if (!stream) return;
    const audioTrack = stream.getAudioTracks()[0];
    if (audioTrack) {
      audioTrack.enabled = !audioTrack.enabled;
      setIsMuted(!audioTrack.enabled);
    }
  }, []);

  return {
    status,
    transcript,
    audioLevel,
    isMuted,
    connect,
    disconnect,
    toggleMute,
  };
}
