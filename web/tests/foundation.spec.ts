import { expect, test } from "@playwright/test";

test("serves the embedded application and health endpoint", async ({
  page,
  request,
}) => {
  const health = await request.get("/api/v1/health/live");
  expect(health.ok()).toBe(true);
  await expect(health.json()).resolves.toEqual({ status: "ok" });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Portinaia" })).toBeVisible();

  await page.goto("/inventory");
  await expect(page.getByRole("heading", { name: "Portinaia" })).toBeVisible();
});
