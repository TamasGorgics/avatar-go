import { defineConfig, devices } from "@playwright/test";

const appPort = 18080;
const fakePort = 19099;
const appURL = `http://127.0.0.1:${appPort}`;
const fakeURL = `http://127.0.0.1:${fakePort}`;

function testEnv(overrides: Record<string, string>): Record<string, string> {
  const env: Record<string, string> = {};
  for (const [key, value] of Object.entries({ ...process.env, ...overrides })) {
    if (value !== undefined) {
      env[key] = value;
    }
  }
  return env;
}

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI
    ? [["github"], ["html", { open: "never" }]]
    : [["list"], ["html", { open: "never" }]],
  outputDir: "test-results",
  use: {
    baseURL: appURL,
    reducedMotion: "reduce",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "node e2e/support/fake-openai.mjs",
      url: `${fakeURL}/healthz`,
      reuseExistingServer: false,
      timeout: 30_000,
      env: testEnv({ FAKE_OPENAI_PORT: String(fakePort) }),
    },
    {
      command: "node e2e/support/start-app.mjs",
      url: `${appURL}/healthz`,
      reuseExistingServer: false,
      timeout: 120_000,
      gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
      env: testEnv({
        ADDR: `127.0.0.1:${appPort}`,
        OPENAI_API_KEY: "e2e-key",
        LLM_BASE_URL: `${fakeURL}/v1`,
        LLM_MODEL: "e2e-model",
        PER_IP_MINUTE: "1000",
        PER_IP_HOURLY: "1000",
        PER_IP_DAILY_TOKENS: "10000000",
        GLOBAL_MINUTE: "1000",
        DAILY_CAP: "10000",
        PER_IP_CONCURRENT: "8",
        GLOBAL_CONCURRENT: "16",
        LOG_LEVEL: "error",
      }),
    },
  ],
});
