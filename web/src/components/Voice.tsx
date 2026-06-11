// components/Voice.tsx — voice control zone.
//
// V1.1: push-to-talk automatico via VAD (Voice Activity Detection).
// L'utente parla → VAD rileva → registra → fine silenzio → invia a
// STT → mostra transcript → esegue comando → TTS risposta.
//
// P7-b: toggle "Continuous" (wake-word). Quando attivo, ogni utterance
// passa da /v1/voice/stt e poi POST /v1/voice/command con
// {text, continuous:true}; se il server risponde ignored=true (nessuna
// wake word) mostriamo solo un hint discreto invece di una risposta.
//
// Barge-in: se TTS sta parlando e l'utente inizia a parlare, il VAD
// chiama onSpeechStart → interrompiamo l'audio TTS in corso.

import { useCallback, useEffect, useRef, useState } from "react";
import { useVAD } from "../hooks/useVAD";
import { stt, speak, voiceCommand } from "../lib/api";

type HistoryEntry = {
  text: string;
  action: string;
  response?: string;
  ts: string;
};

export default function Voice() {
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [status, setStatus] = useState<"idle" | "listening" | "processing" | "speaking">("idle");
  const [error, setError] = useState<string | null>(null);
  const [continuous, setContinuous] = useState(false); // P7-b wake-word mode
  const [hint, setHint] = useState(false); // ignored=true → subtle hint
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const continuousRef = useRef(continuous);
  continuousRef.current = continuous;

  const stopTTS = useCallback(() => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
      audioRef.current = null;
    }
    abortRef.current?.abort();
  }, []);

  const playTTS = useCallback(async (text: string) => {
    try {
      setStatus("speaking");
      const blob = await speak(text);
      const url = URL.createObjectURL(blob);
      const audioEl = new Audio(url);
      audioRef.current = audioEl;
      const done = () => {
        URL.revokeObjectURL(url);
        if (audioRef.current === audioEl) audioRef.current = null;
        setStatus("idle");
      };
      audioEl.onended = done;
      audioEl.onerror = done;
      await audioEl.play();
    } catch {
      setStatus("idle"); // TTS is best-effort
    }
  }, []);

  const onSpeechEnd = useCallback(async (audio: Float32Array) => {
    setStatus("processing");
    const isContinuous = continuousRef.current;
    try {
      // Convert Float32Array PCM 16kHz mono → WAV Blob for the server
      const wav = float32ToWav(audio, 16000);
      const blob = new Blob([wav], { type: "audio/wav" });

      abortRef.current = new AbortController();

      // 1. STT
      const transcript = (await stt(blob)) || "";
      if (!transcript.trim()) {
        if (isContinuous) setHint(true);
        setStatus("idle");
        return;
      }

      // 2. Command dispatch (wake-word gated when continuous).
      const res = await voiceCommand(transcript, isContinuous, abortRef.current.signal);

      // No wake word in continuous mode → subtle hint, no response.
      if (isContinuous && res.ignored) {
        setHint(true);
        setStatus("idle");
        return;
      }
      setHint(false);

      const entry: HistoryEntry = {
        text: res.heard || transcript || "(vuoto)",
        action: res.action || "—",
        response: res.text_response,
        ts: new Date().toLocaleTimeString(),
      };
      setHistory((h) => [entry, ...h].slice(0, 10));

      // 3. TTS della risposta (barge-in interrompibile)
      if (res.text_response) {
        await playTTS(res.text_response);
      } else {
        setStatus("idle");
      }
    } catch (e: unknown) {
      const msg = (e as Error)?.message || "fail";
      if (msg !== "AbortError" && !msg.includes("abort")) {
        setHistory((h) => [{ text: msg, action: "error", ts: new Date().toLocaleTimeString() }, ...h].slice(0, 10));
      }
      setStatus("idle");
    }
  }, [playTTS]);

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
    <div className="flex flex-col h-full gap-2 p-3">
      <header className="flex items-center justify-between">
        <h2 className="text-xs font-semibold text-[#ededed] uppercase tracking-wider">Voice</h2>
        <span className={`text-[10px] ${speaking ? "text-emerald-400" : ready ? "text-amber-300" : "text-zinc-500"}`}>
          {status === "idle" && (ready && continuous ? "● in ascolto (wake word)" : "● in attesa")}
          {status === "listening" && "● ascolto…"}
          {status === "processing" && "● elaboro…"}
          {status === "speaking" && "● rispondo…"}
        </span>
      </header>

      {/* Continuous (wake-word) toggle — default off */}
      <button
        onClick={() => {
          setContinuous((c) => !c);
          setHint(false);
        }}
        className="flex items-center justify-between text-[10px] px-2 py-1.5 rounded border transition-colors"
        style={{
          background: continuous ? "rgba(0,212,170,0.10)" : "#161718",
          borderColor: continuous ? "rgba(0,212,170,0.35)" : "rgba(255,255,255,0.06)",
          color: continuous ? "#00D4AA" : "#888",
        }}
        title="Wake-word mode: esegue solo i comandi che iniziano con 'bismuth …'"
      >
        <span>continuous · wake word</span>
        <span
          className="inline-flex w-6 h-3.5 rounded-full p-0.5 transition-colors"
          style={{ background: continuous ? "#00D4AA" : "#333" }}
        >
          <span
            className="w-2.5 h-2.5 rounded-full bg-[#08090a] transition-transform"
            style={{ transform: continuous ? "translateX(10px)" : "translateX(0)" }}
          />
        </span>
      </button>

      {error && <p className="text-xs text-rose-400">{error}</p>}

      <div className="flex items-center gap-3">
        <button
          onClick={toggle}
          className={`w-14 h-14 shrink-0 rounded-full flex items-center justify-center text-xl transition-colors ${
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
        {/* Wake-word hint (ignored=true: niente risposta, solo ascolto) */}
        {continuous && hint && (
          <p className="text-[10px] text-zinc-500 italic">
            …in ascolto — di' «bismuth …»
          </p>
        )}
      </div>

      <div className="flex-1 overflow-y-auto space-y-1 min-h-0">
        {history.map((h, i) => (
          <div key={i} className="text-xs bg-zinc-900 rounded p-2">
            <p className="text-zinc-300">{h.text}</p>
            {h.response && (
              <p className="text-[10px] text-emerald-300/80 mt-0.5">{h.response}</p>
            )}
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
