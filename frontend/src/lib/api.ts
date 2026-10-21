// Thin HTTP client for the Go backend. All requests go to the same origin
// (/api is proxied by next.config.ts), so the session cookie is sent
// automatically and unsafe requests carry the double-submit CSRF token.

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

const CSRF_COOKIE = "csrf_token";
const UNSAFE_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);

export function readCookie(name: string, cookieString: string = document.cookie): string | null {
  for (const part of cookieString.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return decodeURIComponent(v.join("="));
  }
  return null;
}

// Ensures a CSRF cookie exists; the backend sets one on any request.
async function ensureCsrfToken(): Promise<string> {
  let token = readCookie(CSRF_COOKIE);
  if (!token) {
    await fetch("/api/health", { credentials: "same-origin" });
    token = readCookie(CSRF_COOKIE);
  }
  return token ?? "";
}

export interface RequestOptions {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
}

export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const method = (opts.method ?? "GET").toUpperCase();
  const headers: Record<string, string> = { Accept: "application/json" };
  let body: BodyInit | undefined;
  if (opts.body instanceof FormData) {
    body = opts.body; // browser sets the multipart boundary
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  if (UNSAFE_METHODS.has(method)) {
    headers["X-CSRF-Token"] = await ensureCsrfToken();
  }

  const res = await fetch(path, { method, headers, body, credentials: "same-origin", signal: opts.signal });
  if (res.status === 204) return undefined as T;

  const data = await res.json().catch(() => null);
  if (!res.ok) {
    const err = data?.error;
    throw new ApiError(res.status, err?.code ?? "UNKNOWN", err?.message ?? "發生未知錯誤");
  }
  return data as T;
}

export const api = {
  get: <T>(path: string, signal?: AbortSignal) => apiRequest<T>(path, { signal }),
  post: <T>(path: string, body?: unknown) => apiRequest<T>(path, { method: "POST", body }),
  put: <T>(path: string, body?: unknown) => apiRequest<T>(path, { method: "PUT", body }),
  del: <T>(path: string, body?: unknown) => apiRequest<T>(path, { method: "DELETE", body }),
};
