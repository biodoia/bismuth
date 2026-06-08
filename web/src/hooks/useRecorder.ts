// hooks/useRecorder.ts — MediaRecorder API wrapper for voice capture.
//
// Captures 16kHz mono audio in webm/opus (Chrome/Firefox) or webm/ogg
// (Safari). Returns a recorder object with start/stop/getBlob methods.
//
// On stop, the recorded blob is auto-uploaded to /v1/voice/stt and
// the transcript is returned via onTranscript callback.

import { useRef, useState, useCallback } from "react";

export type RecorderState = "idle" | "recording" | "transcribing" | "error";

export function useRecorder(onTranscript: (text: string) => void) {
  const [state, setState] = useState<RecorderState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [audioLevel, setAudioLevel] = useState(0); // 0..1
  const recRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const levelTimerRef = useRef<number | null>(null);

  const start = useCallback(async () => {
    if (recRef.current && recRef.current.state === "recording") return;
    setError(null);
    chunksRef.current = [];
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          sampleRate: 16000,
          echoCancellation: true,
          noiseSuppression: true,
        },
      });
      streamRef.current = stream;
      const mime = pickMime();
      const rec = new MediaRecorder(stream, { mimeType: mime });
      recRef.current = rec;
      rec.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data);
      };
      rec.onstop = async () => {
        cleanup();
        setState("transcribing");
        try {
          const blob = new Blob(chunksRef.current, { type: mime });
          const fd = new FormData();
          fd.append("file", blob, "audio.webm");
          fd.append("lang", "it");
          const r = await fetch("/v1/voice/stt", { method: "POST", body: fd });
          if (!r.ok) throw new Error("stt " + r.status);
          const { text } = await r.json();
          onTranscript(text);
          setState("idle");
        } catch (e: any) {
          setError(e.message || "transcribe failed");
          setState("error");
        }
      };
      rec.start(250); // 250ms chunks
      setState("recording");
      // VU meter
      audioCtxRef.current = new AudioContext();
      const src = audioCtxRef.current.createMediaStreamSource(stream);
      analyserRef.current = audioCtxRef.current.createAnalyser();
      analyserRef.current.fftSize = 256;
      src.connect(analyserRef.current);
      const data = new Uint8Array(analyserRef.current.frequencyBinCount);
      const tick = () => {
        if (!analyserRef.current) return;
        analyserRef.current.getByteFrequencyData(data);
        let sum = 0;
        for (let i = 0; i < data.length; i++) sum += data[i];
        setAudioLevel(Math.min(1, sum / (data.length * 255)));
        levelTimerRef.current = requestAnimationFrame(tick);
      };
      tick();
    } catch (e: any) {
      setError(e.message || "mic error");
      setState("error");
    }
  }, [onTranscript]);

  const stop = useCallback(() => {
    if (recRef.current && recRef.current.state === "recording") {
      recRef.current.stop();
    }
  }, []);

  const cleanup = useCallback(() => {
    if (levelTimerRef.current) cancelAnimationFrame(levelTimerRef.current);
    levelTimerRef.current = null;
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
    if (audioCtxRef.current) {
      audioCtxRef.current.close();
      audioCtxRef.current = null;
    }
    analyserRef.current = null;
    recRef.current = null;
    setAudioLevel(0);
  }, []);

  return { state, error, audioLevel, start, stop };
}

function pickMime(): string {
  const cands = [
    "audio/webm;codecs=opus",
    "audio/webm",
    "audio/ogg;codecs=opus",
    "audio/mp4",
  ];
  for (const m of cands) {
    if (typeof MediaRecorder !== "undefined" && MediaRecorder.isTypeSupported(m)) {
      return m;
    }
  }
  return "audio/webm";
}
