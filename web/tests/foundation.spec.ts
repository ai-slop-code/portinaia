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
  await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
  await expect(page.getByText("Add your first host")).toBeVisible();

  // Guided flow: domain → provider (as its registrar) → place → host → agent.
  await page.getByRole("link", { name: "Add", exact: true }).click();
  await page.getByLabel("Domain name").fill("e2e.example");
  await page.getByRole("button", { name: "Continue" }).click();

  await page.getByLabel("Provider name").fill("E2E Hosting Provider");
  await page.getByLabel("Registrar").check();
  await expect(
    page.getByLabel("Use as registrar for e2e.example"),
  ).toBeChecked();
  await page.getByRole("button", { name: "Continue" }).click();

  await page.getByLabel("Name", { exact: true }).fill("E2E Datacenter");
  await expect(
    page.getByRole("combobox", { name: "Provider", exact: true }),
  ).toHaveText("E2E Hosting Provider");
  await page.getByRole("button", { name: "Continue" }).click();

  await page.getByLabel("Hostname").fill("E2E Physical Host");
  await page.getByRole("radio", { name: /Physical/ }).check();
  await expect(
    page.getByRole("combobox", { name: "Place", exact: true }),
  ).toHaveText("E2E Datacenter");
  await page.getByRole("button", { name: "Create compute" }).click();

  await page
    .getByRole("button", { name: "Create one-time enrollment token" })
    .click();
  await expect(
    page.getByLabel("One-time enrollment token", { exact: true }),
  ).toHaveText(/.+/);
  await expect(
    page.getByLabel("Install commands", { exact: true }),
  ).toContainText("portinaia-agent enroll");
  await page.getByRole("button", { name: "Revoke unused token" }).click();
  await expect(
    page.getByLabel("One-time enrollment token", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: "Open host" }).click();
  await expect(
    page.getByRole("heading", { name: "E2E Physical Host" }),
  ).toBeVisible();

  // A VM runs on the physical host.
  await page.goto("/add?step=compute");
  await page.getByLabel("Hostname").fill("E2E Virtual Machine");
  await page.getByRole("radio", { name: /VM/ }).check();
  await page.getByRole("combobox", { name: "Runs on", exact: true }).click();
  await page.getByRole("option", { name: "E2E Physical Host" }).click();
  await page.getByRole("button", { name: "Create compute" }).click();
  await page.getByRole("link", { name: "Open host" }).click();
  await expect(page.getByText("on E2E Physical Host")).toBeVisible();

  await page.getByRole("tab", { name: "Network" }).click();
  await page.getByRole("button", { name: "Add IP address" }).click();
  await page.getByRole("textbox", { name: "Address" }).fill("192.0.2.44");
  await page.getByRole("button", { name: "Add IP assignment" }).click();
  await expect(page.getByText("192.0.2.44")).toBeVisible();

  await page
    .getByRole("navigation", { name: "Main" })
    .getByRole("link", { name: "Compute" })
    .click();
  await expect(
    page.getByRole("link", { name: "Open E2E Physical Host" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open E2E Virtual Machine" }),
  ).toBeVisible();
  await page
    .getByRole("navigation", { name: "Main" })
    .getByRole("link", { name: "Overview" })
    .click();
  await expect(page.getByText("1 VM · 1 physical · 0 VPS")).toBeVisible();

  // Domains: quick add, then fill details in the side panel.
  await page.goto("/domains");
  await expect(
    page.getByRole("link", { name: "Open e2e.example" }),
  ).toContainText("E2E Hosting Provider");
  await page.getByLabel("Domain name").fill("unknown.example");
  await page.getByRole("button", { name: "Add domain" }).click();
  const domainPanel = page.getByTestId("domain-workspace");
  await expect(domainPanel.getByText("Not tracked").first()).toBeVisible();
  await domainPanel.getByRole("button", { name: "Edit" }).click();
  await domainPanel.getByLabel("Expires on").fill("2028-02-03");
  await domainPanel
    .getByRole("combobox", { name: "Auto-renewal", exact: true })
    .click();
  await page.getByRole("option", { name: "No", exact: true }).click();
  await domainPanel.getByRole("button", { name: "Save domain" }).click();
  await expect(domainPanel.getByText("2028-02-03")).toBeVisible();
  await domainPanel.getByRole("button", { name: "Archive" }).click();
  await expect(
    domainPanel.getByRole("button", { name: "Restore" }),
  ).toBeVisible();
  await domainPanel.getByRole("button", { name: "Restore" }).click();
  await expect(
    domainPanel.getByRole("button", { name: "Archive" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Close" }).click();

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
  // Lists read every page, so nothing hides behind "load more".
  await page.reload();
  await expect(
    page.getByRole("link", { name: "Open bulk-050.example" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open unknown.example" }),
  ).toBeVisible();
  await page.goto(`/compute/${physicalHost.id}?tab=network`);
  await expect(page.getByText("198.51.100.100/24")).toBeVisible();
  await expect(page.getByText("198.51.100.150/24")).toBeVisible();

  await page.goto("/providers");
  await page.getByRole("link", { name: "Open E2E Hosting Provider" }).click();
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
  await page.goto("/places");
  await expect(
    page.getByRole("link", { name: "Open E2E Datacenter" }),
  ).toHaveCount(0);
  await page.getByLabel("Show archived").check();
  await expect(
    page.getByRole("link", { name: "Open E2E Datacenter" }),
  ).toContainText("Archived");
  await page.getByRole("link", { name: "Open E2E Datacenter" }).click();
  await expect(
    page.getByRole("link", { name: /E2E Physical Host/ }),
  ).toBeVisible();

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

  await page.goto("/compute");
  await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open E2E Virtual Machine" }),
  ).toBeVisible();
  await page.getByLabel("Show archived").check();
  await expect(
    page.getByRole("link", { name: "Open E2E Physical Host" }),
  ).toContainText("Archived");
  await page.goto("/domains");
  await expect(
    page.getByRole("link", { name: "Open bulk-000.example" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open e2e.example" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open unknown.example" }),
  ).toBeVisible();
  await page.goto("/compute");
  await page.getByRole("link", { name: "Open E2E Virtual Machine" }).click();
  await page.getByRole("tab", { name: "Network" }).click();
  await expect(page.getByText("192.0.2.44")).toBeVisible();
});
