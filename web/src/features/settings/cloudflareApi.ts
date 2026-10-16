import { request } from "../../api";
import type { CloudflareStatus } from "../../types";

const base = "/api/v1/remote/cloudflare";

export function getCloudflare() {
  return request<CloudflareStatus>(base);
}

export function startCloudflareLogin(purpose: "setup" | "remove" = "setup") {
  return request<CloudflareStatus>(`${base}/login`, { method: "POST", body: JSON.stringify({ purpose }) });
}

export function cancelCloudflareLogin() {
  return request<CloudflareStatus>(`${base}/login`, { method: "DELETE" });
}

export function setupCloudflare(body: { label: string; overwrite?: boolean }) {
  return request<CloudflareStatus>(`${base}/setup`, { method: "POST", body: JSON.stringify(body) });
}

export function turnOffCloudflare() {
  return request<CloudflareStatus>(base, { method: "DELETE" });
}
