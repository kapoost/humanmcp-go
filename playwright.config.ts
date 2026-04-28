import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 15000,
  use: {
    baseURL: process.env.BASE_URL || 'https://kapoost-humanmcp.fly.dev',
    headless: true,
  },
});
