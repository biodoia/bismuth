// hooks/useVAD.ts — Voice Activity Detection via @ricky0123/vad-web
//
// Runs entirely in-browser (WebAssembly, no server). Detects speech
// segments and calls onSpeechStart / onSpeechEnd with the audio blob.
//
// Usage:
//   const { start, stop, speaking, ready } = useVAD({
//     onSpeechStart: () => setStatus("speaking"),
//     onSpeechEnd: (audio) => sendToSTT(audio),
//   });

import { useCallback, useEffect, useRef, useState } from "react";
import { MicVAD } from "@ricky0123/vad-web";

interface VADOptions {
  onSpeechStart?: () => void;
  onSpeechEnd?: (audio: Float32Array) => void;
  onError?: (err: Error) => void;
}

export function useVAD(opts: VADOptions) {
  const [ready, setReady] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const vadRef = useRef<MicVAD | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const start = useCallback(async () => {
    try {
      const vad = await MicVAD.new({
        onSpeechStart: () => {
          setSpeaking(true);
          optsRef.current.onSpeechStart?.();
        },
        onSpeechEnd: (audio) => {
          setSpeaking(false);
          optsRef.current.onSpeechEnd?.(audio);
        },
      });
      vadRef.current = vad;
      vad.start();
      setReady(true);
    } catch (e) {
      optsRef.current.onError?.(e as Error);
    }
  }, []);

  const stop = useCallback(() => {
    vadRef.current?.pause();
    vadRef.current = null;
    setReady(false);
    setSpeaking(false);
  }, []);

  useEffect(() => {
    return () => stop();
  }, [stop]);

  return { start, stop, speaking, ready };
}
