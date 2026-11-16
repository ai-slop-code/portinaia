import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";

import { App } from "./App";

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          type: "about:blank",
          title: "Unauthorized",
          status: 401,
          code: "unauthorized",
          request_id: "req_test",
        }),
        {
          status: 401,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
    ),
  );
});

test("renders the sign-in screen", async () => {
  render(<App />);

  expect(
    screen.getByRole("heading", { name: "Portinaia" }),
  ).toBeInTheDocument();
  expect(
    await screen.findByRole("button", { name: "Sign in" }),
  ).toBeInTheDocument();
});
