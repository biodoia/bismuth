// hooks/useAgentStream.ts — live agent output via SSE (P7-j).
//
// Consumes GET /api/v1/agents/:id/stream with native EventSource:
//
//   event: output  data: {"chunk_b64":"<base64 bytes>"}
//   event: state   data: {"state":"working|idle|exited"}
//   : ping                       (heartbeat comment, ignored by spec)
//
// On `state` events the agent's badge in the zustand store is patched.
// On EventSource error the stream is closed and status flips to
// "error" so the caller (Terminal) can fall back to /read polling.
// Selecting another agent (or re-mounting) retries SSE from scratch.

import { useEffect, useRef, useState } from "react";
import { useBismuthStore } from "../lib/store";

export type AgentStreamStatus = "idle" | "connecting" | "open" | "error";

export interface AgentStreamHandlers {
  onOutput?: (bytes: Uint8Array) => void;
  onState?: (state: string) => void;
}

export function useAgentStream(
  agentId: string | null,
  handlers: AgentStreamHandlers = {}
): { status: AgentStreamStatus } {
  const [status, setStatus] = useState<AgentStreamStatus>("idle");
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;
  const updateAgentState = useBismuthStore((s) => s.updateAgentState);

  useEffect(() => {
    if (!agentId) {
      setStatus("idle");
      return;
    }
    setStatus("connecting");
    const es = new EventSource(
      `/api/v1/agents/${encodeURIComponent(agentId)}/stream`
    );

    es.onopen = () => setStatus("open");

    es.addEventListener("output", (ev) => {
      try {
        const { chunk_b64 } = JSON.parse((ev as MessageEvent).data);
        if (typeof chunk_b64 === "string" && chunk_b64.length > 0) {
          handlersRef.current.onOutput?.(b64ToBytes(chunk_b64));
        }
      } catch {
        /* malformed frame — skip */
      }
    });

    es.addEventListener("state", (ev) => {
      try {
        const { state } = JSON.parse((ev as MessageEvent).data);
        if (typeof state === "string" && state) {
          updateAgentState(agentId, state);
          handlersRef.current.onState?.(state);
        }
      } catch {
        /* malformed frame — skip */
      }
    });

    // Contract: error → close + let the caller fall back to polling.
    // (Also fires when the server replies 404/409, e.g. pane gone.)
    es.onerror = () => {
      es.close();
      setStatus("error");
    };

    return () => {
      es.close();
      setStatus("idle");
    };
  }, [agentId, updateAgentState]);

  return { status };
}

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const arr = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
  return arr;
}
