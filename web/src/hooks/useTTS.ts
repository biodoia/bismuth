// hooks/useTTS.ts — text-to-speech via bismuth /v1/voice/speak.
// Plays the returned audio blob. Caches nothing (bismuth uses Edge TTS,
// which is cheap to call again).

import { useCallback, useState } from "react";

export function useTTS() {
  const [speaking, setSpeaking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const audioRef = new Audio();

  const speak = useCallback(async (text: string) => {
    if (!text.trim()) return;
    setError(null);
    setSpeaking(true);
    try {
      const r = await fetch("/v1/voice/speak", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text }),
      });
      if (!r.ok) throw new Error("speak " + r.status);
      const { audio_b64, format } = await r.json();
      const bin = atob(audio_b64);
      const arr = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
      const blob = new Blob([arr], { type: `audio/${format}` });
      const url = URL.createObjectURL(blob);
      audioRef.src = url;
      audioRef.onended = () => {
        setSpeaking(false);
        URL.revokeObjectURL(url);
      };
      audioRef.onerror = () => {
        setSpeaking(false);
        setError("audio playback failed");
        URL.revokeObjectURL(url);
      };
      await audioRef.play();
    } catch (e: any) {
      setError(e.message || "speak failed");
      setSpeaking(false);
    }
  }, []);

  return { speak, speaking, error };
}
