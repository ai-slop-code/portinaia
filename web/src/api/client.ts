import createClient from "openapi-fetch";

import type { paths } from "./generated";

export const apiClient = createClient<paths>({
  baseUrl: `${window.location.origin}/api/v1`,
  credentials: "include",
  fetch: (...args) => apiFetch(...args),
});

let csrfToken: string | undefined;
let sessionGeneration = 0;
let sessionController = new AbortController();

export type ApiProblem = {
  title?: string;
  detail?: string;
  code?: string;
  request_id?: string;
  errors?: Record<string, string[]>;
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly problem?: ApiProblem,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class SessionChangedError extends Error {
  constructor() {
    super("The session changed while this request was in progress.");
    this.name = "SessionChangedError";
  }
}

/** A stable server-state key namespace, always scoped to the active session. */
export function sessionQueryKey<T extends readonly unknown[]>(
  resource: string,
  ...identity: T
) {
  return ["session", sessionGeneration, resource, ...identity] as const;
}

/** Bind edits to the identity and revision from which their values were loaded. */
export type ResourceFormSnapshot<T> = Readonly<{
  id: string;
  etag: string;
  initialValues: T;
}>;

export function resourceFormSnapshot<T>(
  id: string,
  etag: string,
  initialValues: T,
): ResourceFormSnapshot<T> {
  return { id, etag, initialValues };
}

export function currentSessionGeneration() {
  return sessionGeneration;
}

export function setCSRFToken(value: string | undefined) {
  if (csrfToken !== value) {
    csrfToken = value;
    sessionGeneration += 1;
    sessionController.abort();
    sessionController = new AbortController();
    if (typeof window !== "undefined") {
      window.dispatchEvent(new Event("portinaia:session-changed"));
    }
    return;
  }
  csrfToken = value;
}

export async function readApiError(response: Response): Promise<ApiError> {
  let problem: ApiProblem | undefined;
  try {
    problem = (await response.clone().json()) as ApiProblem;
  } catch {
    // Non-problem responses still retain their HTTP status.
  }
  return new ApiError(
    problem?.detail ?? problem?.title ?? `Request failed (${response.status}).`,
    response.status,
    problem,
  );
}

export async function apiFetch(
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<Response> {
  const generation = sessionGeneration;
  const requestSignal = input instanceof Request ? input.signal : undefined;
  const signals = [requestSignal, init.signal, sessionController.signal].filter(
    (signal): signal is AbortSignal => signal !== undefined,
  );
  const signal = AbortSignal.any(signals);
  const request = new Request(input, {
    ...init,
    signal,
    credentials: "include",
  });
  const method = request.method.toUpperCase();
  if (csrfToken && !["GET", "HEAD", "OPTIONS"].includes(method)) {
    request.headers.set("X-CSRF-Token", csrfToken);
  }
  if (
    !["GET", "HEAD", "OPTIONS"].includes(method) &&
    !request.headers.has("Idempotency-Key")
  ) {
    request.headers.set("Idempotency-Key", idempotencyKey());
  }
  const response = await fetch(request);
  if (generation !== sessionGeneration) throw new SessionChangedError();
  if (response.status === 401 && typeof window !== "undefined") {
    window.dispatchEvent(new Event("portinaia:session-expired"));
  }
  // openapi-fetch decodes after its fetch implementation resolves. Keep the
  // originating session attached through that asynchronous body read as well.
  const assertCurrent = () => {
    if (generation !== sessionGeneration) throw new SessionChangedError();
  };
  for (const method of [
    "json",
    "text",
    "arrayBuffer",
    "blob",
    "formData",
  ] as const) {
    const decode = response[method].bind(response);
    Object.defineProperty(response, method, {
      configurable: true,
      value: async (...args: Parameters<Response[typeof method]>) => {
        assertCurrent();
        const value = await decode(...args);
        assertCurrent();
        return value;
      },
    });
  }
  return response;
}

function idempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  const bytes = new Uint8Array(16);
  globalThis.crypto?.getRandomValues(bytes);
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}
