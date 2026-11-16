import { describe, expect, it } from "vitest";
import type { components } from "../../api/generated";
import {
  domainEditSnapshot,
  domainWriteFromForm,
  mergeRegistrarOptions,
  normalizeDomainError,
} from "./domainAdapter";

type Domain = components["schemas"]["Domain"];
const domain = {
  id: "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73c0",
  revision: 3,
  name: "example.test",
  registrar_provider_id: "registrar-id",
  registered_on: undefined,
  expires_on: undefined,
  auto_renew: null,
  nameservers: [],
  tag_ids: ["tag-id"],
  description: "",
  notes: "",
  expiry_state: "unknown",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  archived_at: undefined,
} as unknown as Domain;

describe("domain adapter", () => {
  it("captures an immutable identity and original revision ETag", () => {
    const snapshot = domainEditSnapshot(domain);
    expect(snapshot).toEqual(
      expect.objectContaining({
        id: domain.id,
        etag: '"revision-3"',
        preservedTagIDs: ["tag-id"],
        initialValues: expect.objectContaining({
          name: "example.test",
          auto_renew: "unknown",
        }),
      }),
    );
    expect(Object.isFrozen(snapshot)).toBe(true);
    expect(Object.isFrozen(snapshot.initialValues)).toBe(true);
    expect(Object.isFrozen(snapshot.preservedTagIDs)).toBe(true);
  });

  it("keeps unknown date and renewal values and preserves existing tags", () => {
    const form = new FormData();
    form.set("name", " Example.Test ");
    form.set("auto_renew", "unknown");
    form.set("nameservers", "ns1.example.test\nns2.example.test");
    expect(domainWriteFromForm(form, domain)).toMatchObject({
      name: "Example.Test",
      auto_renew: null,
      nameservers: ["ns1.example.test", "ns2.example.test"],
      tag_ids: ["tag-id"],
    });
    const empty = domainWriteFromForm(new FormData(), domain);
    expect(empty.registered_on).toBeUndefined();
    expect(empty.expires_on).toBeUndefined();
  });

  it("retains the selected registrar outside the current search page", () => {
    const registrar = {
      id: "registrar-id",
      name: "Registrar",
      roles: ["registrar"] as components["schemas"]["Provider"]["roles"],
    };
    expect(mergeRegistrarOptions([], registrar.id, registrar)).toEqual([
      registrar,
    ]);
  });

  it("normalizes duplicate, invalid-domain, and stale-revision errors", () => {
    expect(normalizeDomainError(409, "duplicate_domain")).toContain(
      "already exists",
    );
    expect(normalizeDomainError(422, "invalid_domain")).toContain(
      "valid DNS domain",
    );
    expect(normalizeDomainError(412)).toContain("edits are preserved");
  });
});
