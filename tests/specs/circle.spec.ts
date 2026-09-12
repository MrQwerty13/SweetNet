import { test, expect, type Page } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import YAML from 'yaml';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

const password = process.env.E2E_PASSWORD!;
const origin = process.env.E2E_BASE_URL!;
const image = readFileSync(new URL('../fixtures/lake.png', import.meta.url));
async function login(page: Page, username: string, pass = password) {
 await page.goto('/login');
 await page.getByLabel('Логин', { exact: true }).fill(username);
 await page.getByLabel('Пароль', { exact: true }).fill(pass);
 await page.getByRole('button', { name: 'Войти', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'Наш круг' })).toBeVisible();
}
test('two friends, protected photos, profiles, membership and mobile UI', async ({ browser, page }) => {
 const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
 await login(page, process.env.E2E_USERNAME!);
 await page.getByRole('link', { name: 'Приглашения', exact: true }).click();
 await page.getByRole('button', { name: /Создать приглашение/ }).click();
 const link = await page.getByLabel(/Ссылка приглашения/).inputValue();
 expect(link).toContain('/join#token=');
 const friendContext = await browser.newContext({ viewport: { width: 360, height: 800 } });
 const friend = await friendContext.newPage(); friend.on('pageerror', e => errors.push(e.message));
 await friend.goto(link);
 await expect(friend.getByText(/Публикации доступны всем участникам/)).toBeVisible();
 await friend.getByLabel('Логин', { exact: true }).fill('anna');
 await friend.getByLabel('Отображаемое имя', { exact: true }).fill('Анна');
 await friend.getByLabel('Пароль', { exact: true }).fill(password);
 await friend.getByRole('button', { name: 'Присоединиться' }).click();
 await expect(friend.getByRole('heading', { name: 'Наш круг' })).toBeVisible();
 await expect(friend.getByRole('link', { name: 'Участники', exact: true })).toHaveCount(0);
 await page.getByRole('link', { name: 'Лента', exact: true }).click();
 await page.getByRole('link', { name: /Новый пост/ }).click();
 await page.getByLabel(/Текст/).fill('В субботу идём гулять. Кто с нами?');
 await page.locator('input[type=file]').setInputFiles({ name: 'lake.png', mimeType: 'image/png', buffer: image });
 await page.getByRole('button', { name: /Опубликовать/ }).click();
 await expect(page.getByRole('heading', { name: 'Публикация', exact: true })).toBeVisible();
 await expect(page.getByText('В субботу идём гулять. Кто с нами?', { exact: true })).toBeVisible();
 await friend.reload();
 await expect(friend.getByText('В субботу идём гулять. Кто с нами?', { exact: true })).toBeVisible();
 await expect(friend.locator('article img').first()).toBeVisible();
 expect(await friend.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await friend.screenshot({ path: '../.local/mobile-feed.png', fullPage: true });
 // Direct APIs agree with the UI and never expose media to anonymous clients.
 const feedResponse = await page.request.get('/api/v1/posts'); expect(feedResponse.status()).toBe(200);
 const feed = await feedResponse.json(); const post = feed.items[0];
 const anonymous = await browser.newContext(); const guest = await anonymous.newPage();
 for (const path of ['/api/v1/posts', `/api/v1/posts/${post.id}`, post.images[0].url]) {
  expect((await anonymous.request.get(origin + path)).status()).toBe(401);
 }
 expect((await friend.request.patch(`/api/v1/posts/${post.id}`, { headers: { Origin: origin }, data: { body: 'changed' } })).status()).toBe(403);
 expect((await friend.request.delete(`/api/v1/posts/${post.id}`, { headers: { Origin: origin } })).status()).toBe(403);
 await friend.getByRole('link', { name: /Новый пост/ }).click();
 await friend.getByLabel(/Текст/).fill('Как здорово, что теперь у нас есть своё место.');
 await friend.getByRole('button', { name: /Опубликовать/ }).click();
 await expect(friend.getByText('Как здорово, что теперь у нас есть своё место.', { exact: true })).toBeVisible();
 await page.goto('/'); await expect(page.locator('article img').first()).toBeVisible(); await page.screenshot({ path: '../.local/desktop-feed.png', fullPage: true });
 await friend.getByRole('link', { name: 'Профиль', exact: true }).click();
 await friend.getByLabel('Отображаемое имя', { exact: true }).fill('Анна Петрова');
 await friend.getByRole('button', { name: /Сохранить имя/ }).click();
 await expect(friend.getByText(/Имя сохранено/)).toBeVisible();
 await page.getByRole('link', { name: 'Участники', exact: true }).click();
 const annaRow = page.locator('article, tr, li').filter({ hasText: '@anna' });
 await annaRow.getByRole('button', { name: /Отключить/ }).click();
 // Confirmation may be native dialog; UI uses a second explicit confirmation button.
 const confirmDisable = page.getByRole('dialog').getByRole('button', { name: 'Отключить', exact: true });
 if (await confirmDisable.isVisible()) await confirmDisable.click();
 await friend.goto('/');
 await expect(friend.getByRole('heading', { name: 'Войти в SweetNet' })).toBeVisible();
 await expect(friend.getByText('В субботу идём гулять. Кто с нами?', { exact: true })).toHaveCount(0);
 await page.getByRole('button', { name: 'Выйти', exact: true }).click();
 await expect(page.getByRole('heading', { name: 'Войти в SweetNet' })).toBeVisible();
 await page.goBack();
 await expect(page.getByText('В субботу идём гулять. Кто с нами?', { exact: true })).toHaveCount(0);
 await guest.goto('/'); await expect(guest.getByRole('heading', { name: 'Войти в SweetNet' })).toBeVisible();
 expect(errors).toEqual([]);
 await friendContext.close(); await anonymous.close();
});

test('responses satisfy OpenAPI and offline reset revokes sessions', async ({ request }) => {
 const spec = YAML.parse(readFileSync(new URL('../../api/openapi.yaml', import.meta.url), 'utf8'));
 const ajv = new Ajv2020({ strict: false }); addFormats(ajv);
 const validate = (name: string, data: unknown) => {
  const check = ajv.compile({ $ref: `#/components/schemas/${name}`, components: spec.components });
  expect(check(data), JSON.stringify(check.errors)).toBe(true);
 };
 const signed = await request.post('/api/v1/auth/login', { headers: { Origin: origin }, data: { username: 'owner', password } }); expect(signed.status()).toBe(200); validate('User', await signed.json());
 const me = await request.get('/api/v1/me'); validate('User', await me.json());
 validate('Feed', await (await request.get('/api/v1/posts')).json());
 const invitation = await request.post('/api/v1/admin/invites', { headers: { Origin: origin } }); validate('CreatedInvite', await invitation.json());
 validate('Invites', await (await request.get('/api/v1/admin/invites')).json());
 validate('Users', await (await request.get('/api/v1/admin/users')).json());
 const denied = await request.patch('/api/v1/me', { headers: { Origin: 'https://evil.example' }, data: { display_name: 'bad' } }); expect(denied.status()).toBe(403); validate('Error', await denied.json());
 const reset = spawnSync(new URL('../../.local/admin', import.meta.url).pathname, ['reset-password', '--username', 'owner'], { env: process.env, input: randomBytes(20).toString('hex') + '\n', encoding: 'utf8' });
 expect(reset.status, reset.stderr).toBe(0);
 expect((await request.get('/api/v1/me')).status()).toBe(401);
 const restorePassword = spawnSync(new URL('../../.local/admin', import.meta.url).pathname, ['reset-password', '--username', 'owner'], { env: process.env, input: password + '\n', encoding: 'utf8' });
 expect(restorePassword.status).toBe(0);
});
