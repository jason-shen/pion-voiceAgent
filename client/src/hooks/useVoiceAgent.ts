"use client";

import { useCallback, useRef, useState } from "react";
import { SignalingClient, type SignalingMessage } from "../lib/signaling";

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

const SIGNALING_URL =
  process.env.NEXT_PUBLIC_SIGNALING_URL ?? "ws://localhost:8080/ws";

export function useVoiceAgent(): UseVoiceAgentReturn {
  const [status, setStatus] = useState<ConnectionStatus>("idle");
  const [transcript, setTranscript] = useState<TranscriptEntry[]>([]);
  const [audioLevel, setAudioLevel] = useState(0);
  const [isMuted, setIsMuted] = useState(false);

  const signalingRef = useRef<SignalingClient | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
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

  const handleSignalingMessage = useCallback(
    (
      msg: SignalingMessage,
      pc: RTCPeerConnection,
      pendingCandidates: RTCIceCandidateInit[]
    ) => {
      switch (msg.type) {
        case "answer": {
          const desc = new RTCSessionDescription({
            type: "answer",
            sdp: msg.sdp,
          });
          pc.setRemoteDescription(desc)
            .then(() =>
              Promise.all(
                pendingCandidates.map((c) =>
                  pc.addIceCandidate(new RTCIceCandidate(c))
                )
              )
            )
            .then(() => pendingCandidates.splice(0))
            .catch(console.error);
          break;
        }
        case "candidate": {
          if ("candidate" in msg && msg.candidate) {
            if (pc.remoteDescription) {
              pc.addIceCandidate(new RTCIceCandidate(msg.candidate)).catch(
                console.error
              );
            } else {
              pendingCandidates.push(msg.candidate);
            }
          }
          break;
        }
        case "transcript": {
          if (msg.final) {
            setTranscript((prev) => {
              const updated = prev.filter(
                (e) => !(e.role === "user" && e.partial)
              );
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

        const signaling = new SignalingClient(SIGNALING_URL);
        signalingRef.current = signaling;
        await signaling.connect();

        const pc = new RTCPeerConnection({
          iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
        });
        pcRef.current = pc;

        const pendingCandidates: RTCIceCandidateInit[] = [];
        const unsubscribe = signaling.onMessage((msg) =>
          handleSignalingMessage(msg, pc, pendingCandidates)
        );

        pc.onicecandidate = (e) => {
          if (e.candidate) {
            signaling.send({
              type: "candidate",
              candidate: e.candidate.toJSON(),
            });
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

        signaling.send({ type: "join", room });

        await new Promise<void>((resolve) => {
          const unsub = signaling.onMessage((msg) => {
            if (msg.type === "joined") {
              unsub();
              resolve();
            }
          });
        });

        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        signaling.send({ type: "offer", sdp: offer.sdp! });

        signaling.onMessage((msg) => {
          if (
            msg.type === "transcript" &&
            msg.final &&
            assistantBufRef.current
          ) {
            const finalText = assistantBufRef.current;
            assistantBufRef.current = "";
            setTranscript((prev) => {
              const updated = prev.filter(
                (e) => !(e.role === "assistant" && e.partial)
              );
              return [...updated, { role: "assistant", text: finalText }];
            });
          }
        });

        pc.addEventListener("close", () => unsubscribe());
      } catch (err) {
        console.error("[voiceagent] connect error:", err);
        setStatus("error");
      }
    },
    [handleSignalingMessage, startAudioLevelMonitoring, cleanupAudioLevel]
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

    pcRef.current?.close();
    pcRef.current = null;
    signalingRef.current?.close();
    signalingRef.current = null;
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
