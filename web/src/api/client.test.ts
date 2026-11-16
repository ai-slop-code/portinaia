import { afterEach, expect, test, vi } from "vitest";

import {
  apiFetch,
  currentSessionGeneration,
  readApiError,
  SessionChangedError,
  setCSRFToken,
} from "./client";

afterEach(() => {
  setCSRFToken(undefined);
  vi.unstubAllGlobals();
});

test("sends credentials and the CSRF token on mutations", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);
  setCSRFToken("csrf-test");

  const response = await apiFetch(
    new URL("/api/v1/providers", window.location.origin),
    {
      method: "POST",
    },
  );

  const request = fetchMock.mock.calls[0][0] as Request;
  expect(response.status).toBe(204);
  expect(request.credentials).toBe("include");
  expect(request.headers.get("X-CSRF-Token")).toBe("csrf-test");
  expect(request.headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{32,36}$/);
});

test("preserves a caller-provided idempotency key", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);

  await apiFetch(new URL("/api/v1/providers", window.location.origin), {
    method: "POST",
    headers: { "Idempotency-Key": "caller-retry-key" },
  });

  const request = fetchMock.mock.calls[0][0] as Request;
  expect(request.headers.get("Idempotency-Key")).toBe("caller-retry-key");
});

test("notifies the app when an API request expires the session", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 401 }));
  vi.stubGlobal("fetch", fetchMock);
  const expired = vi.fn();
  window.addEventListener("portinaia:session-expired", expired);

  try {
    const response = await apiFetch(
      new URL("/api/v1/providers", window.location.origin),
    );

    expect(response.status).toBe(401);
    expect(expired).toHaveBeenCalledOnce();
  } finally {
    window.removeEventListener("portinaia:session-expired", expired);
  }
});

test("session replacement aborts and rejects responses from the old session", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>((resolve) => (finish = resolve))),
  );
  setCSRFToken("first-session");
  const generation = currentSessionGeneration();
  const pending = apiFetch(
    new URL("/api/v1/providers", window.location.origin),
  );

  setCSRFToken("replacement-session");
  expect(currentSessionGeneration()).toBe(generation + 1);
  finish(new Response(null, { status: 200 }));
  await expect(pending).rejects.toBeInstanceOf(SessionChangedError);
});

test("preserves the signal on an incoming Request alongside session cancellation", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);
  const controller = new AbortController();
  const request = new Request(
    new URL("/api/v1/providers", window.location.origin),
    { signal: controller.signal },
  );

  await apiFetch(request);

  const forwarded = fetchMock.mock.calls[0][0] as Request;
  controller.abort();
  expect(forwarded.signal.aborted).toBe(true);
});

test("rejects a decoded body when the session changes after headers", async () => {
  let finish!: (value: unknown) => void;
  const response = new Response(null, { status: 200 });
  response.json = () => new Promise((resolve) => (finish = resolve));
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
  setCSRFToken("body-session");

  const pendingResponse = await apiFetch(
    new URL("/api/v1/providers", window.location.origin),
  );
  const pendingBody = pendingResponse.json();
  setCSRFToken("replacement-body-session");
  finish({ items: [] });

  await expect(pendingBody).rejects.toBeInstanceOf(SessionChangedError);
});

test("normalizes problem responses with request and field error details", async () => {
  const error = await readApiError(
    new Response(
      JSON.stringify({
        title: "Conflict",
        detail: "The resource changed.",
        code: "stale_revision",
        request_id: "req_123",
        errors: { name: ["Already used"] },
      }),
      { status: 412, headers: { "Content-Type": "application/problem+json" } },
    ),
  );

  expect(error.status).toBe(412);
  expect(error.message).toBe("The resource changed.");
  expect(error.problem).toMatchObject({
    code: "stale_revision",
    request_id: "req_123",
    errors: { name: ["Already used"] },
  });
});
