import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 30_000,
	fullyParallel: true,
	reporter: [['list'], ['html', { open: 'never' }]],
	use: {
		baseURL: process.env.E2E_BASE_URL || 'http://localhost:5173',
		trace: 'on-first-retry',
	},
	projects: [
		{ name: 'chromium', use: { ...devices['Desktop Chrome'] } },
	],
	webServer: process.env.E2E_BASE_URL
		? undefined
		: {
				command: 'npm run dev',
				url: 'http://localhost:5173',
				reuseExistingServer: !process.env.CI,
				timeout: 60_000,
			},
});
