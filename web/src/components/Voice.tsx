// components/Voice.tsx — voice control zone (left column of App).
//
// V1.1: push-to-talk automatico via VAD (Voice Activity Detection).
// L'utente parla → VAD rileva → registra → fine silenzio → invia a
// STT → mostra transcript → esegue comando → TTS risposta.
//
// Barge-in: se TTS sta parlando e l'utente inizia a parlare, il VAD
// chiama onSpeechStart → interrompiamo l'audio TTS in corso.

import { useCallback, useEffect, useRef, useState } from "react";
import { useVAD } from "../hooks/useVAD";
import { voiceCommand } from "../lib/api";

export default function Voice() {
  const [history, setHistory] = useState<{ text: string; action: string; ts: string }[]>([]);
  const [status, setStatus] = useState<"idle" | "listening" | "processing" | "speaking">("idle");
  const [error, setError] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const stopTTS = useCallback(() => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
      audioRef.current = null;
    }
    abortRef.current?.abort();
  }, []);

  const onSpeechEnd = useCallback(async (audio: Float32Array) => {
    setStatus("processing");
    try {
      // Convert Float32Array PCM 16kHz mono → WAV Blob for the server
      const wav = float32ToWav(audio, 16000);
      const form = new FormData();
      form.append("audio", new Blob([wav], { type: "audio/wav" }), "voice.wav");

      abortRef.current = new AbortController();
      const res = await voiceCommand(form, abortRef.current.signal);

      const entry = { text: res.transcript || "(vuoto)", action: res.action || "—", ts: new Date().toLocaleTimeString() };
      setHistory((h) => [entry, ...h].slice(0, 10));

      if (res.audio_url) {
        setStatus("speaking");
        const audioEl = new Audio(res.audio_url);
        audioRef.current = audioEl;
        audioEl.onended = () => {
          audioRef.current = null;
          setStatus("idle");
        };
        audioEl.play();
      } else {
        setStatus("idle");
      }
    } catch (e: unknown) {
      const msg = (e as Error)?.message || "fail";
      setHistory((h) => [{ text: msg, action: "error", ts: new Date().toLocaleTimeString() }, ...h].slice(0, 10));
      setStatus("idle");
    }
  }, []);

  const onSpeechStart = useCallback(() => {
    setStatus("listening");
    // Barge-in: se TTS sta parlando, interrompi
    stopTTS();
  }, [stopTTS]);

  const { start, stop, speaking, ready } = useVAD({
    onSpeechStart,
    onSpeechEnd,
    onError: (err) => setError(err.message),
  });

  const toggle = () => {
    if (status === "idle" || status === "speaking") {
      start();
    } else {
      stop();
      setStatus("idle");
    }
  };

  // Space bar toggle
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code === "Space" && !["INPUT", "TEXTAREA"].includes((e.target as HTMLElement)?.tagName || "")) {
        e.preventDefault();
        toggle();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle]);

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Voce</h2>
        <span className={`text-[10px] ${speaking ? "text-emerald-400" : ready ? "text-amber-300" : "text-zinc-500"}`}>
          {status === "idle" && "● in attesa"}
          {status === "listening" && "● ascolto…"}
          {status === "processing" && "● elaboro…"}
          {status === "speaking" && "● rispondo…"}
        </span>
      </header>

      {error && <p className="text-xs text-rose-400">{error}</p>}

      <button
        onClick={toggle}
        className={`w-16 h-16 rounded-full flex items-center justify-center text-2xl transition-colors ${
          speaking
            ? "bg-emerald-500/20 text-emerald-400 animate-pulse"
            : ready
            ? "bg-amber-500/20 text-amber-300"
            : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
        }`}
        title="Spazio per attivare"
      >
        {speaking ? "🎙️" : "🎤"}
      </button>

      <div className="flex-1 overflow-y-auto space-y-1">
        {history.map((h, i) => (
          <div key={i} className="text-xs bg-zinc-900 rounded p-2">
            <p className="text-zinc-300">{h.text}</p>
            <p className="text-[10px] text-zinc-500 mt-1">
              → {h.action} · {h.ts}
            </p>
          </div>
        ))}
      </div>

      <p className="text-[10px] text-zinc-600">
        VAD auto · Spazio toggle · barge-in attivo
      </p>
    </div>
  );
}

// float32ToWav converte un buffer PCM mono Float32 in WAV Blob.
function float32ToWav(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);

  const writeString = (offset: number, str: string) => {
    for (let i = 0; i < str.length; i++) {
      view.setUint8(offset + i, str.charCodeAt(i));
    }
  };

  writeString(0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeString(8, "WAVE");
  writeString(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeString(36, "data");
  view.setUint32(40, samples.length * 2, true);

  let offset = 44;
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
    offset += 2;
  }

  return buffer;
}
