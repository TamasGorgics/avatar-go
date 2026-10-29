import { expect, test } from "@playwright/test";

test("theme follows the system preference and persists an explicit choice", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/");

  const html = page.locator("html");
  const toggle = page.getByRole("switch");
  const themeColor = page.locator('meta[name="theme-color"]');

  await expect(html).toHaveAttribute("data-theme", "dark");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await expect(themeColor).toHaveAttribute("content", "#0e171c");

  await toggle.click();
  await expect(html).toHaveAttribute("data-theme", "light");
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await expect(themeColor).toHaveAttribute("content", "#fbfaf7");
  expect(await page.evaluate(() => localStorage.getItem("theme"))).toBe("light");

  await page.reload();
  await expect(html).toHaveAttribute("data-theme", "light");
  await expect(toggle).toHaveAttribute("aria-checked", "false");

  await toggle.click();
  await expect(html).toHaveAttribute("data-theme", "dark");
  expect(await page.evaluate(() => localStorage.getItem("theme"))).toBe("dark");
});
