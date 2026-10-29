import { expect, test } from "@playwright/test";

test.describe("mobile layout", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("fits the viewport and can complete a question", async ({ page }) => {
    await page.goto("/");

    const input = page.getByLabel("Your question");
    const submit = page.getByRole("button", { name: "Ask", exact: true });
    await expect(input).toBeVisible();
    await expect(submit).toBeVisible();
    await expect(page.getByRole("switch")).toBeVisible();

    const before = await page.evaluate(() => ({
      horizontal: document.documentElement.scrollWidth - window.innerWidth,
      vertical: document.documentElement.scrollHeight - window.innerHeight,
    }));
    expect(before.horizontal).toBeLessThanOrEqual(1);
    expect(before.vertical).toBeLessThanOrEqual(1);

    await input.fill("What are you working on now?");
    await submit.click();

    const assistant = page.locator('#chat-log [data-role="message"]').last();
    await expect(assistant).toHaveAttribute("data-state", "complete");
    await expect(assistant.locator('[data-role="text"]')).toHaveText(
      "I build Go services and developer tools.",
    );

    const after = await page.evaluate(() => ({
      horizontal: document.documentElement.scrollWidth - window.innerWidth,
      vertical: document.documentElement.scrollHeight - window.innerHeight,
    }));
    expect(after.horizontal).toBeLessThanOrEqual(1);
    expect(after.vertical).toBeLessThanOrEqual(1);
  });
});
