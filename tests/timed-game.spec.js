import { test, expect } from "@playwright/test";

test("the 'Timed Game' button should be clickable", async ({ page }) => {
  await page.goto("http://localhost:8080/");
  await page.getByRole("button", { name: "Timed Game" }).click();
  // Check the game-window div is visible
  await expect(page.locator("#game-window")).toBeVisible();
});

test("the 'skip this probelem' button should be clickable", async ({
  page,
}) => {
  await page.goto("http://localhost:8080/");
  await page.getByRole("button", { name: "Timed Game" }).click();
  await page.getByRole("button", { name: "Skip This Problem" }).click();
  // Expect the game-window div to be visible
  await expect(page.locator("#game-window")).toBeVisible();
});

test("the 'end game' button should be clickable", async ({ page }) => {
  await page.goto("http://localhost:8080/");
  await page.getByRole("button", { name: "Timed Game" }).click();
  await page.getByRole("button", { name: "End Game" }).click();
  // Expect the ending-window div to be visible
  await expect(page.locator("#ending-window")).toBeVisible();
});
