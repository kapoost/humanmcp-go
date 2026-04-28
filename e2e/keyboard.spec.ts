import { test, expect } from '@playwright/test';

const BASE = process.env.BASE_URL || 'https://kapoost-humanmcp.fly.dev';

test.describe('keyboard shortcuts — index', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE);
  });

  test('footer shows index hints [/] [j/k] [1-3]', async ({ page }) => {
    const hints = page.locator('#kb-hints');
    await expect(hints).toContainText('[/]');
    await expect(hints).toContainText('[j/k]');
    await expect(hints).toContainText('[1-3]');
    await expect(hints).not.toContainText('[b]');
  });

  test('[/] opens search box', async ({ page }) => {
    const searchBox = page.locator('#search-box');
    await expect(searchBox).not.toBeVisible();
    await page.keyboard.press('/');
    await expect(searchBox).toBeVisible();
  });

  test('[Escape] closes search box', async ({ page }) => {
    await page.keyboard.press('/');
    await expect(page.locator('#search-box')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('#search-box')).not.toBeVisible();
  });

  test('[?] toggles help overlay', async ({ page }) => {
    const help = page.locator('#help-overlay');
    await expect(help).not.toBeVisible();
    await page.keyboard.press('?');
    await expect(help).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(help).not.toBeVisible();
  });

  test('[j] and [k] navigate items', async ({ page }) => {
    const items = page.locator('.navigable');
    const count = await items.count();
    if (count === 0) return;

    await page.keyboard.press('j');
    await expect(items.first()).toHaveClass(/active/);

    await page.keyboard.press('j');
    if (count > 1) {
      await expect(items.nth(1)).toHaveClass(/active/);
      await expect(items.first()).not.toHaveClass(/active/);
    }

    await page.keyboard.press('k');
    await expect(items.first()).toHaveClass(/active/);
  });

  test('[1] scrolls to #wiersze', async ({ page }) => {
    await page.keyboard.press('1');
    await page.waitForTimeout(500);
    const visible = await page.locator('#wiersze').isVisible();
    expect(visible).toBe(true);
  });

  test('[2] scrolls to #obrazy', async ({ page }) => {
    await page.keyboard.press('2');
    await page.waitForTimeout(500);
    const visible = await page.locator('#obrazy').isVisible();
    expect(visible).toBe(true);
  });

  test('[3] scrolls to #ogloszenia', async ({ page }) => {
    await page.keyboard.press('3');
    await page.waitForTimeout(500);
    const visible = await page.locator('#ogloszenia').isVisible();
    expect(visible).toBe(true);
  });

  test('[d] toggles theme', async ({ page }) => {
    await page.keyboard.press('d');
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBeTruthy();
    const first = theme;

    await page.keyboard.press('d');
    const second = await page.locator('html').getAttribute('data-theme');
    expect(second).not.toBe(first);
  });

  test('[c] navigates to /connect', async ({ page }) => {
    await page.keyboard.press('c');
    await page.waitForURL('**/connect');
    expect(page.url()).toContain('/connect');
  });

  test('[m] navigates to /contact', async ({ page }) => {
    await page.keyboard.press('m');
    await page.waitForURL('**/contact');
    expect(page.url()).toContain('/contact');
  });

  test('[Enter] opens selected item', async ({ page }) => {
    const items = page.locator('.navigable');
    const count = await items.count();
    if (count === 0) return;

    await page.keyboard.press('j');
    const href = await items.first().getAttribute('data-href');
    await page.keyboard.press('Enter');
    await page.waitForURL(`**${href}`);
    expect(page.url()).toContain(href!);
  });
});

test.describe('keyboard shortcuts — piece page', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to first piece via index
    await page.goto(BASE);
    const firstLink = page.locator('.irc-title a').first();
    const count = await firstLink.count();
    if (count === 0) return;
    await firstLink.click();
    await page.waitForLoadState('domcontentloaded');
  });

  test('footer shows piece hints [b] and [d]', async ({ page }) => {
    const hints = page.locator('#kb-hints');
    await expect(hints).toContainText('[b]');
    await expect(hints).toContainText('[d]');
    await expect(hints).not.toContainText('[/]');
    await expect(hints).not.toContainText('[j/k]');
  });

  test('[b] navigates back to index', async ({ page }) => {
    await page.keyboard.press('b');
    await page.waitForURL(BASE + '/');
    expect(page.url()).toBe(BASE + '/');
  });

  test('[d] toggles theme on piece page', async ({ page }) => {
    await page.keyboard.press('d');
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBeTruthy();
  });
});

test.describe('keyboard shortcuts — listing page', () => {
  test('footer shows [b] on listing page', async ({ page }) => {
    await page.goto(BASE + '/listings');
    const hints = page.locator('#kb-hints');
    await expect(hints).toContainText('[b]');
    await expect(hints).not.toContainText('[/]');
  });
});
