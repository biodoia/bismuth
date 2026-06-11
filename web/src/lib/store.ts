// lib/store.ts — zustand store shared across zones.
//
// Holds the agent + task lists, exposes fetch actions (polled by App /
// panels), live agent-state patches from the SSE stream (P7-j), and a
// tiny toast queue used for drag-and-drop assignment feedback (P7-i).

import { create } from "zustand";
import { listAgents, listTasks } from "./api";
import type { Agent, Task } from "./types";

export type Toast = {
  id: number;
  msg: string;
  kind: "success" | "error" | "info";
};

interface BismuthState {
  agents: Agent[];
  tasks: Task[];
  toast: Toast | null;

  setAgents: (agents: Agent[]) => void;
  fetchAgents: () => Promise<void>;
  fetchTasks: () => Promise<void>;
  // Patch a single agent's state badge (SSE `state` events).
  updateAgentState: (id: string, state: string) => void;
  showToast: (msg: string, kind?: Toast["kind"]) => void;
}

let toastSeq = 0;
let toastTimer: ReturnType<typeof setTimeout> | null = null;

export const useBismuthStore = create<BismuthState>()((set) => ({
  agents: [],
  tasks: [],
  toast: null,

  setAgents: (agents) => set({ agents }),

  fetchAgents: async () => {
    try {
      const body = await listAgents();
      set({ agents: (body.agents || []) as Agent[] });
    } catch {
      /* server unreachable — keep last known list */
    }
  },

  fetchTasks: async () => {
    try {
      const body = await listTasks();
      set({ tasks: (body.tasks || []) as Task[] });
    } catch {
      /* ignore */
    }
  },

  updateAgentState: (id, state) =>
    set((s) => ({
      agents: s.agents.map((a) =>
        a.id === id ? { ...a, state: state as Agent["state"] } : a
      ),
    })),

  showToast: (msg, kind = "info") => {
    if (toastTimer) clearTimeout(toastTimer);
    const t: Toast = { id: ++toastSeq, msg, kind };
    set({ toast: t });
    toastTimer = setTimeout(() => {
      set((s) => (s.toast?.id === t.id ? { toast: null } : s));
    }, 3500);
  },
}));
