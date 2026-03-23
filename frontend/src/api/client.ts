const API_BASE = "/api/v1";

let accessToken = "";
let unauthorizedHandler: (() => void) | null = null;

export function setAccessToken(token: string) {
  accessToken = token;
}

export function setUnauthorizedHandler(handler: (() => void) | null) {
  unauthorizedHandler = handler;
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...(options.headers ?? {}),
    },
  });

  if (!response.ok) {
    const payload = await response.text();
    if (response.status === 401) {
      accessToken = "";
      unauthorizedHandler?.();
    }
    throw new Error(extractErrorMessage(payload, response.statusText));
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  if (!text) {
    return undefined as T;
  }

  return JSON.parse(text) as T;
}

export function extractErrorMessage(payload: string, fallback: string) {
  const trimmed = payload.trim();
  if (!trimmed) {
    return humanizeErrorMessage(fallback);
  }

  try {
    const parsed = JSON.parse(trimmed) as { error?: unknown; message?: unknown };
    if (typeof parsed.error === "string" && parsed.error.trim()) {
      return humanizeErrorMessage(parsed.error.trim());
    }
    if (typeof parsed.message === "string" && parsed.message.trim()) {
      return humanizeErrorMessage(parsed.message.trim());
    }
  } catch {
    // Fall back to the raw response body when it is not JSON.
  }

  return humanizeErrorMessage(trimmed);
}

function humanizeErrorMessage(value: string) {
  const message = value.trim();
  if (!message) {
    return message;
  }
  return message.charAt(0).toUpperCase() + message.slice(1);
}
