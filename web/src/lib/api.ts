// lib/api.ts — minimal REST client for bismuth server.

const base = ""; // vite proxy handles it

export async function listAgents() {
  const r = await fetch(`${base}/api/v1/agents`);
  if (!r.ok) throw new Error(`listAgents ${r.status}`);
  return r.json();
}

export async function spawnAgent(opts: { role: string; cli: string; task: string; args?: string[] }) {
  const r = await fetch(`${base}/api/v1/agents`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(opts),
  });
  if (!r.ok) throw new Error(`spawnAgent ${r.status}`);
  return r.json();
}

export async function sendToAgent(agentId: string, data: string) {
  const r = await fetch(`${base}/api/v1/agents/${agentId}/send`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ data_b64: btoa(data) }),
  });
  if (!r.ok) throw new Error(`sendToAgent ${r.status}`);
  return r.json();
}

export async function readAgent(agentId: string, n = 200) {
  const r = await fetch(`${base}/api/v1/agents/${agentId}/read?n=${n}`);
  if (!r.ok) throw new Error(`readAgent ${r.status}`);
  return r.json();
}

export async function killAgent(agentId: string) {
  const r = await fetch(`${base}/api/v1/agents/${agentId}/kill`, { method: "POST" });
  if (!r.ok) throw new Error(`killAgent ${r.status}`);
  return r.json();
}

export async function stt(audio: Blob, lang = "it"): Promise<string> {
  const fd = new FormData();
  fd.append("file", audio, "audio.webm");
  fd.append("lang", lang);
  const r = await fetch(`${base}/v1/voice/stt`, { method: "POST", body: fd });
  if (!r.ok) throw new Error(`stt ${r.status}`);
  return (await r.json()).text;
}

export async function speak(text: string): Promise<Blob> {
  const r = await fetch(`${base}/v1/voice/speak`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
  if (!r.ok) throw new Error(`speak ${r.status}`);
  const { audio_b64, format } = await r.json();
  const bin = atob(audio_b64);
  const arr = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
  return new Blob([arr], { type: `audio/${format}` });
}
