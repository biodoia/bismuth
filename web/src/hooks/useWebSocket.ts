// hooks/useWebSocket.ts — client for /api/v1/ws
// TODO(sessione+1): implement reconnect, sub filters, JSON encode/decode.

import { useEffect, useRef, useState } from "react";

export function useWebSocket(url: string) {
  const [connected, setConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<any>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const ws = new WebSocket(url);
    wsRef.current = ws;
    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onmessage = (e) => {
      try {
        setLastMessage(JSON.parse(e.data));
      } catch {
        setLastMessage(e.data);
      }
    };
    return () => ws.close();
  }, [url]);

  return { connected, lastMessage, send: (m: any) => wsRef.current?.send(JSON.stringify(m)) };
}
