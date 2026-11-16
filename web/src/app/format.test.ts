import { expect, test } from "vitest";

import type { AuditEvent } from "./data";
import { daysUntil, describeAuditEvent, domainExpiryText } from "./format";

const event = (overrides: Partial<AuditEvent>): AuditEvent => ({
  id: "01a1e64f-97d7-7c74-90cb-f5f77be2063f",
  sequence: 1,
  occurred_at: "2026-11-16T21:42:18Z",
  actor_type: "administrator",
  actor_id: "01a1e63c-71f3-7211-b3f9-d5d975c7c1fc",
  action: "compute.create",
  resource_type: "compute",
  resource_id: "host-id",
  request_id: "req_1",
  result: "success",
  changes: [],
  ...overrides,
});

const names = (id?: string) =>
  ({ "host-id": "krabica.m8.sk" })[id ?? ""] as string | undefined;

test("describes inventory changes with the resource's name", () => {
  expect(describeAuditEvent(event({}), names).text).toBe(
    "You added host krabica.m8.sk",
  );
  expect(
    describeAuditEvent(event({ action: "compute.archive" }), names).text,
  ).toBe("You archived host krabica.m8.sk");
});

test("names the host an agent enrolled on from its change record", () => {
  const described = describeAuditEvent(
    event({
      action: "agent.enroll",
      resource_type: "agent",
      resource_id: "agent-id",
      actor_type: "system",
      changes: [
        {
          path: "/compute_id",
          before: null,
          after: '"host-id"',
          redacted: false,
        },
      ],
    }),
    names,
  );
  expect(described).toMatchObject({
    text: "Agent enrolled on krabica.m8.sk",
    who: "System",
    category: "agents",
  });
});

test("marks rejected events and falls back for unknown resources", () => {
  expect(
    describeAuditEvent(
      event({ result: "rejected", resource_id: "missing" }),
      names,
    ).text,
  ).toBe("You added a host (rejected)");
});

test("domain expiry text counts down near the date", () => {
  const now = new Date();
  const inTenDays = new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate() + 10,
  );
  const iso = `${inTenDays.getFullYear()}-${String(inTenDays.getMonth() + 1).padStart(2, "0")}-${String(inTenDays.getDate()).padStart(2, "0")}`;
  expect(daysUntil(iso)).toBe(10);
  expect(
    domainExpiryText({ expires_on: iso } as Parameters<
      typeof domainExpiryText
    >[0]),
  ).toBe("Expires in 10 days");
  expect(domainExpiryText({} as Parameters<typeof domainExpiryText>[0])).toBe(
    "Expiry not set",
  );
});
