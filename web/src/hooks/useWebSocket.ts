// hooks/useWebSocket.ts — resilient WS client with auto-reconnect,
// heartbeat, exponential backoff, and event-type filtering.
//
// Usage:
//   const { events, connected, send, subscribe, unsubscribe } =
//     useBismuthWS({ types: ["agent_state", "pane_output"] });
//
//   useEffect(() => {
//     const off = subscribe((evt) => { console.log(evt); });
//     return off;
//   }, [subscribe]);

import { useEffect, useRef, useState, useCallback } from "react";
import type { Event } from "../lib/types";

export type WSOptions = {
  url?: string;            // default: ws(s)://host/api/v1/ws
  types?: string[];        // event types filter
  agentId?: string;        // agent_id filter
  reconnect?: boolean;     // default true
  maxBackoffMs?: number;   // default 30000
};

export function useBismuthWS(opts: WSOptions = {}) {
  const url = opts.url ?? wsURL(opts);
  const [events, setEvents] = useState<Event[]>([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const handlersRef = useRef<Set<(e: Event) => void>>(new Set());
  const wsRef = useRef<WebSocket | null>(null);
  const closedRef = useRef(false);
  const backoffRef = useRef(500);

  const subscribe = useCallback((h: (e: Event) => void) => {
    handlersRef.current.add(h);
    return () => handlersRef.current.delete(h);
  }, []);

  useEffect(() => {
    closedRef.current = false;

    const connect = () => {
      const params = new URLSearchParams();
      if (opts.types?.length) params.set("types", opts.types.join(","));
      if (opts.agentId) params.set("agent_id", opts.agentId);
      const fullURL = params.toString() ? `${url}?${params}` : url;

      const ws = new WebSocket(fullURL);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        setError(null);
        backoffRef.current = 500;
      };

      ws.onclose = (ev) => {
        setConnected(false);
        if (closedRef.current) return;
        if (!opts.reconnect && opts.reconnect !== undefined) return;
        setTimeout(connect, backoffRef.current);
        backoffRef.current = Math.min(backoffRef.current * 2, opts.maxBackoffMs ?? 30000);
      };

      ws.onerror = (ev) => {
        setError("ws error");
      };

      ws.onmessage = (ev) => {
        try {
          const e: Event = JSON.parse(ev.data);
          setEvents((prev) => [...prev.slice(-499), e]); // last 500
          handlersRef.current.forEach((h) => h(e));
        } catch {
          // ignore malformed
        }
      };
    };

    connect();
    return () => {
      closedRef.current = true;
      wsRef.current?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url]);

  const send = useCallback((m: any) => {
    wsRef.current?.send(JSON.stringify(m));
  }, []);

  return { events, connected, error, send, subscribe };
}

function wsURL(_opts: WSOptions): string {
  if (typeof window === "undefined") return "ws://localhost:9000/api/v1/ws";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/v1/ws`;
}
