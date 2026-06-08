// components/Voice.tsx — voice control zone (left column of App).
//
// Push-to-talk button (or hold-space), waveform VU meter, live
// transcript, history of last 10 transcripts. Sends transcript to
// the bismuth server via /v1/voice/command for parsing, then speaks
// back the parsed action confirmation via TTS.

import { useCallback, useState } from "react";
import { useRecorder } from "../hooks/useRecorder";
import { useTTS } from "../hooks/useTTS";
import { speak as apiSpeak } from "../lib/api";

export default function Voice() {
  const [history, setHistory] = useState<{ text: string; action: string; ts: string }[]>([]);
  const onTranscript = useCallback(async (text: string) => {
    if (!text.trim()) return;
    setHistory((h) => [{ text, action: "...", ts: new Date().toISOString() }, ...h].slice(0, 10));
    try {
      const r = await fetch("/v1/voice/command", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text }),
      });
      const data = await r.json();
      setHistory((h) =>
        h.map((e, i) => (i === 0 ? { ...e, action: `${data.action} ${(data.args || []).join(" ")}` } : e))
      );
      // speak back the server's response
      if (data.text_response) {
        // use the page-level TTS so it's shared
        const blob = await apiSpeak(data.text_response);
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        audio.onended = () => URL.revokeObjectURL(url);
        audio.play();
      }
    } catch (e: unknown) {
      setHistory((h) =>
        h.map((x, i) => (i === 0 ? { ...x, action: "error: " + ((e as Error)?.message || "fail") } : x))
      );
    }
  }, []);

  const { state, error, audioLevel, start, stop } = useRecorder(onTranscript);
  const { speaking: ttsSpeaking } = useTTS();

  const toggle = () => {
    if (state === "recording") stop();
    else if (state === "idle" || state === "error") start();
  };

  return (
    <div className="flex flex-col h-full gap-3">
      <header>
        <h2 className="text-sm font-semibold">Voce</h2>
        <p className="text-xs text-zinc-500">push-to-talk, IT</p>
      </header>

      <button
        onClick={toggle}
        aria-label={state === "recording" ? "Ferma registrazione" : "Parla"}
        className={
          "self-center h-24 w-24 rounded-full text-2xl font-bold flex items-center justify-center transition-colors " +
          (state === "recording"
            ? "bg-red-600 animate-pulse"
            : state === "transcribing"
            ? "bg-amber-600"
            : state === "error"
            ? "bg-rose-800"
            : "bg-emerald-700 hover:bg-emerald-600")
        }
      >
        {state === "recording" ? "■" : state === "transcribing" ? "…" : "🎙"}
      </button>

      {state === "recording" && (
        <div className="h-2 bg-zinc-800 rounded">
          <div
            className="h-2 bg-emerald-500 rounded transition-all"
            style={{ width: `${Math.round(audioLevel * 100)}%` }}
          />
        </div>
      )}

      {error && <p className="text-xs text-rose-400">{error}</p>}

      <div className="flex-1 overflow-y-auto">
        <h3 className="text-xs uppercase text-zinc-500 mb-1">storia</h3>
        <ul className="space-y-2">
          {history.length === 0 && <li className="text-xs text-zinc-600">— nessun comando —</li>}
          {history.map((h, i) => (
            <li key={i} className="text-xs bg-zinc-950 border border-zinc-800 rounded p-2">
              <div className="text-zinc-300">{h.text}</div>
              <div className="text-zinc-500 mt-1">→ {h.action}</div>
            </li>
          ))}
        </ul>
      </div>

      {ttsSpeaking && <div className="text-xs text-emerald-400">🔊 riproduco…</div>}
    </div>
  );
}
