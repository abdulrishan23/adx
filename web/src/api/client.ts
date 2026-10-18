import type { ErrorResponse } from "./types.legacy";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

// Vite answers 502–504 itself when the Go server isn't running.
const UNREACHABLE = "Can't reach the Adx server. Start it with make dev.";

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      headers: { "Content-Type": "application/json", ...init?.headers },
    });
  } catch {
    throw new ApiError(0, UNREACHABLE);
  }
  if (res.status >= 502 && res.status <= 504) throw new ApiError(res.status, UNREACHABLE);
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ErrorResponse | null;
    throw new ApiError(res.status, body?.error ?? res.statusText);
  }
  // 204 No Content: the call worked and there's nothing to read.
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const getJSON = <T>(path: string) => request<T>(path);

export const postJSON = <T>(path: string, body: unknown) =>
  request<T>(path, { method: "POST", body: JSON.stringify(body) });

export const putJSON = <T>(path: string, body: unknown) =>
  request<T>(path, { method: "PUT", body: JSON.stringify(body) });

export const patchJSON = <T>(path: string, body: unknown) =>
  request<T>(path, { method: "PATCH", body: JSON.stringify(body) });

// uploadFile sends one file as multipart form data. The browser sets the
// boundary, so the JSON content type must not be sent.
export async function uploadFile<T>(path: string, file: Blob, name = "upload"): Promise<T> {
  const body = new FormData();
  body.append("file", file, name);
  let res: Response;
  try {
    res = await fetch(path, { method: "POST", body });
  } catch {
    throw new ApiError(0, UNREACHABLE);
  }
  if (res.status >= 502 && res.status <= 504) throw new ApiError(res.status, UNREACHABLE);
  if (!res.ok) {
    const err = (await res.json().catch(() => null)) as ErrorResponse | null;
    throw new ApiError(res.status, err?.error ?? res.statusText);
  }
  return (await res.json()) as T;
}

// Some Go handlers read a form body rather than JSON, such as the GoHighLevel
// private token connect, so the token never sits in a JSON log line.
export const postForm = <T>(path: string, fields: Record<string, string>) =>
  request<T>(path, {
    method: "POST",
    body: new URLSearchParams(fields),
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
  });
