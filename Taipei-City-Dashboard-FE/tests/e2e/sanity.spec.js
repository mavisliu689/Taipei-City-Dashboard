import { expect, test } from '@playwright/test';

test.skip('placeholder e2e — replace once eco-assistant ships', async ({ page }) => {
	await page.goto('/');
	await expect(page).toHaveTitle(/.*/);
});
