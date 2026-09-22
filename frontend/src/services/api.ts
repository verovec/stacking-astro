// Centralized fetch wrapper. Stores call these; components never fetch directly.

import type { Health, StageArtifact, SkyPoint } from "@/types";

// Default to a RELATIVE base ("" → same-origin): the app is served behind a reverse proxy that forwards
// /api to the Go engine — nginx in the container image (docker/default.conf.template) and the Vite dev
// proxy (vite.config.ts) in host-dev. Same-origin means tile <img> loads and SSE need no CORS, and a
// failed tile surfaces as its real HTTP status instead of a cross-origin opaque net::ERR (which used to
// mask engine 502s and trigger a retry storm). Set VITE_API_BASE only to point at an engine on another origin.
export const BASE = import.meta.env.VITE_API_BASE || "";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    signal,
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = (await res.json()) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      // keep statusText
    }
    throw new ApiError(res.status, message);
  }
  return (await res.json()) as T;
}

export const apiGet = <T>(path: string, signal?: AbortSignal) =>
  request<T>("GET", path, undefined, signal);
export const apiPost = <T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
) => request<T>("POST", path, body, signal);
export const apiPut = <T>(path: string, body?: unknown) =>
  request<T>("PUT", path, body);
export const apiDelete = <T>(path: string) => request<T>("DELETE", path);

// health returns the engine identity (GET /api/health) — stores cache it; engine.version is "dev"
// for an un-stamped build.
export const health = () => apiGet<Health>("/api/health");

export const fileUrl = (path: string) =>
  `${BASE}/api/file?path=${encodeURIComponent(path)}`;
// thumbUrl is a small server-resized JPEG of an output image — used by the Runs gallery instead of the
// full-resolution PNG so the page loads fast (the full image is fetched only when a run is opened).
export const thumbUrl = (path: string, w?: number) =>
  `${BASE}/api/thumb?path=${encodeURIComponent(path)}${w ? `&w=${w}` : ""}`;
// skyPoint is the map hover lookup: light pollution at a coordinate, plus cached weather when the
// server already holds some. Safe to call while the pointer moves — it never fetches upstream.
export const skyPoint = (lat: number, lon: number, signal?: AbortSignal) =>
  apiGet<SkyPoint>(
    `/api/sky/point?lat=${lat.toFixed(4)}&lon=${lon.toFixed(4)}`,
    signal,
  );

// Full-resolution stage exports of a finished run: list what it preserved, then render one to PNG or
// TIFF. The export returns a path, which is fetched through fileUrl like any other run artifact.
export const jobStages = (jobId: number) =>
  apiGet<{ stages: StageArtifact[] }>(`/api/jobs/${jobId}/stages`);
export const exportJobStage = (
  jobId: number,
  key: string,
  format: "png" | "tif",
) =>
  apiPost<{ path: string }>(`/api/jobs/${jobId}/stages/export`, {
    key,
    format,
  });

export const eventsUrl = (jobId: number) => `${BASE}/api/jobs/${jobId}/events`;
export const agentTurnEventsUrl = (turnId: string) =>
  `${BASE}/api/agent/turns/${turnId}/events`;
export const previewUrl = (path: string, max?: number) =>
  `${BASE}/api/preview?path=${encodeURIComponent(path)}${
    max ? `&max=${max}` : ""
  }`;
