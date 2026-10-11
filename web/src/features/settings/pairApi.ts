import { request } from "../../api";
import type { ClientDevice, PairGrant, PairPoll, PairTicket } from "../../types";

export function listClients() {
  return request<{ devices: ClientDevice[] }>("/api/v1/clients");
}

export function revokeClient(id: string) {
  return request<{ devices: ClientDevice[] }>(`/api/v1/clients/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function startPair(body: { name: string; kind: string; scopes: string[] }) {
  return request<PairTicket>("/api/v1/pair", { method: "POST", body: JSON.stringify(body) });
}

export function createPairCode(body: { kind?: string; scopes: string[] }) {
  return request<PairTicket>("/api/v1/pair/code", { method: "POST", body: JSON.stringify(body) });
}

export function approvePair(body: { code: string; scopes?: string[] }) {
  return request<{ device: ClientDevice }>("/api/v1/pair/approve", { method: "POST", body: JSON.stringify(body) });
}

export function claimPair(body: { code: string; name: string; kind: string }) {
  return request<PairGrant>("/api/v1/pair/claim", { method: "POST", body: JSON.stringify(body) });
}

export function pollPair(id: string, secret: string) {
  return request<PairPoll>(`/api/v1/pair/${encodeURIComponent(id)}?secret=${encodeURIComponent(secret)}`);
}
