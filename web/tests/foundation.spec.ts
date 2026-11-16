import { expect, test } from "@playwright/test";
import { readFile, writeFile } from "node:fs/promises";

test("serves the embedded application and health endpoint", async ({
  page,
  request,
}) => {
  const health = await request.get("/api/v1/health/live");
  expect(health.ok()).toBe(true);
  await expect(health.json()).resolves.toMatchObject({ status: "live" });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Portinaia" })).toBeVisible();

  await page.goto("/inventory");
  await expect(page.getByRole("heading", { name: "Portinaia" })).toBeVisible();
});

test("admin can create inventory and domains that survive a server restart", async ({
  page,
  request,
}, testInfo) => {
  test.setTimeout(120_000);
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("e2e-administrator-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(
    page.getByRole("heading", { name: "Infrastructure inventory" }),
  ).toBeVisible();

  const providerForm = page.getByTestId("provider-create-form");
  await providerForm.locator('input[name="name"]').fill("E2E Hosting Provider");
  await providerForm.getByLabel("Hosting").check();
  await providerForm.getByLabel("Registrar").check();
  await providerForm.getByRole("button", { name: "Create provider" }).click();
  await expect(page.getByText("E2E Hosting Provider").first()).toBeVisible();

  const locationForm = page.getByTestId("location-create-form");
  await locationForm.locator('input[name="name"]').fill("E2E Datacenter");
  await locationForm.getByRole("combobox", { name: "Provider" }).click();
  await page.getByRole("option", { name: "E2E Hosting Provider" }).click();
  await locationForm.getByRole("button", { name: "Create location" }).click();
  await expect(page.getByText("E2E Datacenter").first()).toBeVisible();

  const computeForm = page.getByTestId("compute-create-form");
  await computeForm
    .locator('input[name="display_name"]')
    .fill("E2E Physical Host");
  await computeForm.getByRole("combobox", { name: "Provider" }).click();
  await page.getByRole("option", { name: "E2E Hosting Provider" }).click();
  await computeForm.getByRole("combobox", { name: "Location" }).click();
  await page.getByRole("option", { name: "E2E Datacenter" }).click();
  await computeForm.getByRole("button", { name: "Create compute" }).click();
  await expect(page.getByText("E2E Physical Host").first()).toBeVisible();
  await page
    .getByRole("button", { name: "Create one-time enrollment token" })
    .click();
  await expect(page.getByLabel("One-time enrollment token")).toHaveValue(/.+/);
  await page.getByRole("button", { name: "Revoke unused token" }).click();
  await expect(page.getByLabel("One-time enrollment token")).toHaveCount(0);

  await computeForm
    .locator('input[name="display_name"]')
    .fill("E2E Virtual Machine");
  await computeForm.getByRole("combobox", { name: "Kind" }).click();
  await page.getByRole("option", { name: "VM" }).click();
  await computeForm
    .getByRole("combobox", { name: "Parent compute (VM only)" })
    .click();
  await page.getByRole("option", { name: "E2E Physical Host" }).click();
  await computeForm.getByRole("button", { name: "Create compute" }).click();
  await expect(page.getByText("E2E Virtual Machine").first()).toBeVisible();

  await page
    .getByRole("button", { name: /E2E Datacenter/ })
    .first()
    .click();
  await expect(page.getByText("E2E Physical Host").first()).toBeVisible();
  await page
    .getByRole("button", { name: /E2E Physical Host/ })
    .first()
    .click();
  await expect(page.getByText("E2E Virtual Machine").first()).toBeVisible();
  await expect(page.getByText("Total computes: 2")).toBeVisible();

  await page.getByText("E2E Virtual Machine").first().click();

  await page.getByRole("textbox", { name: "Address" }).fill("192.0.2.44");
  await page.getByRole("button", { name: "Add IP assignment" }).click();
  await expect(page.getByText("192.0.2.44")).toBeVisible();

  const domainForm = page.getByTestId("domain-create-form");
  await domainForm.getByLabel("Domain name").fill("e2e.example");
  await domainForm.getByRole("combobox", { name: "Registrar" }).click();
  await page.getByRole("option", { name: "E2E Hosting Provider" }).click();
  await domainForm.getByRole("button", { name: "Add domain" }).click();
  await expect(page.getByText("e2e.example").first()).toBeVisible();
  await domainForm.getByLabel("Domain name").fill("unknown.example");
  await domainForm.getByRole("button", { name: "Add domain" }).click();
  await expect(page.getByText("unknown.example").first()).toBeVisible();
  await page.getByText("unknown.example").first().click();
  await expect(
    page.getByText("Registered: Unknown · Expires: Unknown"),
  ).toBeVisible();
  await expect(page.getByText("Auto-renew: Unknown")).toBeVisible();
  const domainWorkspace = page.getByTestId("domain-workspace");
  await domainWorkspace.getByRole("button", { name: "Edit" }).click();
  const domainEditForm = domainWorkspace.locator("form").last();
  await domainEditForm.getByLabel("Expires on").fill("2028-02-03");
  await domainEditForm.getByRole("combobox", { name: "Auto-renewal" }).click();
  await page.getByRole("option", { name: "No", exact: true }).click();
  await domainEditForm.getByRole("button", { name: "Save domain" }).click();
  await expect(page.getByText(/Expires: 2028-02-03/)).toBeVisible();
  await expect(page.getByText("Auto-renew: No")).toBeVisible();
  await domainWorkspace.getByRole("button", { name: "Archive" }).click();
  await expect(
    domainWorkspace.getByRole("button", { name: "Restore" }),
  ).toBeVisible();
  await domainWorkspace.getByRole("button", { name: "Restore" }).click();
  await expect(page.getByText(/Expires: 2028-02-03/)).toBeVisible();

  const api = page.context().request;
  const session = await (await api.get("/api/v1/auth/session")).json();
  for (let index = 0; index < 51; index++) {
    const response = await api.post("/api/v1/domains", {
      headers: { "X-CSRF-Token": session.csrf_token },
      data: {
        name: `bulk-${String(index).padStart(3, "0")}.example`,
        nameservers: [],
        tag_ids: [],
      },
    });
    expect(response.status(), `create bulk domain ${index}`).toBe(201);
  }
  const computes = await (
    await api.get("/api/v1/computes?q=E2E%20Physical%20Host")
  ).json();
  const physicalHost = computes.items.find(
    (item: { display_name: string }) =>
      item.display_name === "E2E Physical Host",
  );
  if (!physicalHost)
    throw new Error("Created physical compute was not returned by API");
  for (let index = 100; index <= 150; index++) {
    const response = await api.post("/api/v1/ip-assignments", {
      headers: { "X-CSRF-Token": session.csrf_token },
      data: {
        address: `198.51.100.${index}`,
        prefix_length: 24,
        compute_id: physicalHost.id,
        scope: "private",
        role: "management",
        primary: false,
      },
    });
    expect(response.status(), `create bulk IP ${index}`).toBe(201);
  }
  const archivedToggle = page.getByLabel("Include archived inventory");
  await archivedToggle.check();
  await archivedToggle.uncheck();
  await page.getByText("bulk-000.example", { exact: true }).click();
  await expect(page.getByText("unknown.example", { exact: true })).toHaveCount(
    0,
  );
  await page.getByRole("button", { name: "Load more domains" }).click();
  await expect(
    page.getByText("unknown.example", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /E2E Physical Host PHYSICAL/ })
    .click();
  await expect(page.getByText("198.51.100.100/24")).toBeVisible();
  await expect(page.getByText("198.51.100.150/24")).toHaveCount(0);
  await page.getByRole("button", { name: "Load more IP assignments" }).click();
  await expect(page.getByText("198.51.100.150/24")).toBeVisible();

  await page
    .getByRole("button", { name: /E2E Hosting Provider/ })
    .first()
    .click();
  const providerWorkspace = page.getByTestId("provider-workspace");
  await providerWorkspace.getByRole("button", { name: "Edit" }).click();
  const providerPage = await (
    await api.get("/api/v1/providers?q=E2E%20Hosting%20Provider")
  ).json();
  const provider = providerPage.items.find(
    (item: { name: string }) => item.name === "E2E Hosting Provider",
  );
  if (!provider) throw new Error("Created provider was not returned by API");
  const providerDetail = await api.get(`/api/v1/providers/${provider.id}`);
  const concurrentProviderUpdate = await api.put(
    `/api/v1/providers/${provider.id}`,
    {
      headers: {
        "X-CSRF-Token": session.csrf_token,
        "If-Match": providerDetail.headers()["etag"],
      },
      data: {
        name: provider.name,
        website: provider.website ?? null,
        external_account_reference: provider.external_account_reference ?? null,
        description: "updated in a concurrent browser session",
        notes: provider.notes ?? "",
        roles: provider.roles,
        tag_ids: provider.tag_ids,
      },
    },
  );
  expect(concurrentProviderUpdate.status()).toBe(200);
  const providerEditForm = providerWorkspace.getByTestId("provider-edit-form");
  await providerEditForm
    .getByLabel("Description")
    .fill("preserve this unsaved browser edit");
  await providerEditForm.getByRole("button", { name: "Save provider" }).click();
  await expect(
    page.getByText(
      "Provider changed elsewhere. Reload its details before editing.",
    ),
  ).toBeVisible();
  await expect(providerEditForm.getByLabel("Description")).toHaveValue(
    "preserve this unsaved browser edit",
  );

  const locations = await (
    await api.get("/api/v1/locations?q=E2E%20Datacenter")
  ).json();
  const location = locations.items.find(
    (item: { name: string }) => item.name === "E2E Datacenter",
  );
  if (!location) throw new Error("Created location was not returned by API");
  const locationDetail = await api.get(`/api/v1/locations/${location.id}`);
  const archivedLocation = await api.post(
    `/api/v1/locations/${location.id}/archive`,
    {
      headers: {
        "X-CSRF-Token": session.csrf_token,
        "If-Match": locationDetail.headers()["etag"],
      },
    },
  );
  expect(archivedLocation.ok()).toBe(true);
  const includeArchived = page.getByLabel("Include archived inventory");
  await includeArchived.check();
  await includeArchived.uncheck();
  await expect(page.getByText("location · archived")).toBeVisible();
  await expect(page.getByText("Total computes: 2")).toBeVisible();
  await page
    .getByRole("button", { name: /E2E Datacenter/ })
    .first()
    .click();
  await expect(page.getByText("E2E Physical Host").first()).toBeVisible();

  const hostDetail = await api.get(`/api/v1/computes/${physicalHost.id}`);
  const archivedHost = await api.post(
    `/api/v1/computes/${physicalHost.id}/archive`,
    {
      headers: {
        "X-CSRF-Token": session.csrf_token,
        "If-Match": hostDetail.headers()["etag"],
      },
    },
  );
  expect(archivedHost.ok()).toBe(true);
  const archivedHostChildren = await api.get(
    `/api/v1/inventory/tree/${physicalHost.id}/children`,
  );
  expect(archivedHostChildren.ok()).toBe(true);
  expect(await archivedHostChildren.json()).toMatchObject({
    items: [
      expect.objectContaining({
        kind: "compute",
        name: "E2E Virtual Machine",
        archived: false,
      }),
    ],
  });

  const controlPath = testInfo.config.metadata.e2eControlPath;
  if (typeof controlPath !== "string")
    throw new Error("E2E server control path not configured");
  const firstServerPid = await readFile(`${controlPath}.pid`, "utf8");
  await writeFile(controlPath, "restart");
  await expect
    .poll(
      async () => {
        try {
          return await readFile(`${controlPath}.pid`, "utf8");
        } catch {
          return firstServerPid;
        }
      },
      { timeout: 20_000 },
    )
    .not.toBe(firstServerPid);
  await expect
    .poll(
      async () => {
        try {
          return (await request.get("/api/v1/health/live")).status();
        } catch {
          return 0;
        }
      },
      { timeout: 20_000 },
    )
    .toBe(200);

  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Infrastructure inventory" }),
  ).toBeVisible();
  await expect(page.getByText("E2E Hosting Provider").first()).toBeVisible();
  await page.getByRole("button", { name: "Open E2E Datacenter" }).click();
  await expect(page.getByText("E2E Datacenter").first()).toBeVisible();
  await page.getByRole("button", { name: "Open E2E Physical Host" }).click();
  await expect(page.getByText("E2E Physical Host").first()).toBeVisible();
  await expect(page.getByText("E2E Virtual Machine").first()).toBeVisible();
  await expect(page.getByText("bulk-000.example").first()).toBeVisible();
  await page.getByRole("button", { name: "Load more domains" }).click();
  await expect(page.getByText("e2e.example").first()).toBeVisible();
  await expect(page.getByText("unknown.example").first()).toBeVisible();
  await page.getByRole("button", { name: "Open E2E Physical Host" }).click();
  await page.getByRole("button", { name: "Open E2E Physical Host" }).dblclick();
  await page.getByRole("button", { name: "Open E2E Virtual Machine" }).click();
  await expect(page.getByText("192.0.2.44")).toBeVisible();
});
