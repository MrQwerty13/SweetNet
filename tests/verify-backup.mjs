// Real PostgreSQL + filesystem rehearsal, independent of Docker availability.
// Only databases created by this process are dropped. Source user databases are never touched.
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
import pg from 'pg';
const root = resolve(import.meta.dirname, '..');
const sourceURL = process.env.TEST_DATABASE_URL;
if (!sourceURL) throw new Error('TEST_DATABASE_URL required');
const admin = new pg.Client({ connectionString: sourceURL });
await admin.connect();
const current = await admin.query('SELECT current_database() AS name');
if (!current.rows[0].name.endsWith('_test')) { await admin.end(); throw new Error('Refusing a non-test database'); }
const suffix = randomUUID().replaceAll('-', '').slice(0, 12);
const sourceName = `sweetnet_backup_${suffix}_test`;
const restoredName = `sweetnet_restore_${suffix}_test`;
const created = [];
const temp = await mkdtemp(resolve(tmpdir(), 'sweetnet-backup-'));
const sourceFiles = resolve(temp, 'source-uploads'); const restoredFiles = resolve(temp, 'restored-uploads'); const bundle = resolve(temp, 'bundle');
await Promise.all([mkdir(sourceFiles), mkdir(restoredFiles), mkdir(bundle)]);
const password = randomBytes(24).toString('hex');
const makeURL = name => { const url = new URL(sourceURL); url.pathname = '/' + name; url.searchParams.delete('search_path'); return url.href; };
const envFor = (name, dir, port) => ({ ...process.env, DATABASE_URL: makeURL(name), APP_ENV: 'development', APP_ORIGIN: `http://127.0.0.1:${port}`, HTTP_ADDR: `127.0.0.1:${port}`, UPLOAD_DIR: dir, WEB_DIR: resolve(root, 'web/dist') });
const pgEnvFor = name => {
 const url = new URL(sourceURL);
 return { ...process.env, PGHOST: url.hostname, PGPORT: url.port || '5432', PGUSER: decodeURIComponent(url.username), PGPASSWORD: decodeURIComponent(url.password), PGDATABASE: name, PGSSLMODE: url.searchParams.get('sslmode') || 'prefer' };
};
const sourceEnv = envFor(sourceName, sourceFiles, 18082); const restoredEnv = envFor(restoredName, restoredFiles, 18083);
function command(file, args, env, input) {
 const result = spawnSync(file, args, { env, input, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] });
 if (result.status !== 0) throw new Error(`${file.split('/').at(-1)} ${args[0]} failed (private command output suppressed)`);
 return result;
}
async function start(env) {
 const child = spawn(resolve(root, '.local/server'), [], { env, stdio: 'ignore' });
 child.on('error', () => {});
 for (let i = 0; i < 100; i++) {
  if (child.exitCode !== null) throw new Error('rehearsal server exited');
  try { if ((await fetch(env.APP_ORIGIN + '/readyz')).ok) return child; } catch {}
  await new Promise(r => setTimeout(r, 100));
 }
 child.kill('SIGTERM'); throw new Error('rehearsal readiness timeout');
}
async function stop(child) { if (child && child.exitCode === null) { const done = new Promise(r => child.once('exit', r)); child.kill('SIGTERM'); await done; } }
async function req(env, path, { method = 'GET', data, form, cookie } = {}) {
 const headers = { Origin: env.APP_ORIGIN };
 if (cookie) headers.Cookie = cookie;
 if (data) headers['Content-Type'] = 'application/json';
 return fetch(env.APP_ORIGIN + path, { method, headers, body: form ?? (data ? JSON.stringify(data) : undefined) });
}
async function login(env, username, pass = password) {
 const r = await req(env, '/api/v1/auth/login', { method: 'POST', data: { username, password: pass } });
 assert.equal(r.status, 200); return r.headers.getSetCookie()[0].split(';')[0];
}
let sourceServer, restoredServer;
try {
 for (const name of [sourceName, restoredName]) { await admin.query(`CREATE DATABASE ${name}`); created.push(name); }
 command(resolve(root, '.local/admin'), ['migrate'], sourceEnv);
 command(resolve(root, '.local/admin'), ['bootstrap', '--username', 'owner', '--display-name', 'Владелец'], sourceEnv, password + '\n');
 const repeated = spawnSync(resolve(root, '.local/admin'), ['bootstrap', '--username', 'other', '--display-name', 'Other'], { env: sourceEnv, input: password + '\n', stdio: ['pipe', 'ignore', 'ignore'] });
 assert.notEqual(repeated.status, 0, 'bootstrap must reject a second owner');
 sourceServer = await start(sourceEnv);
 const ownerCookie = await login(sourceEnv, 'owner');
 const inv = await (await req(sourceEnv, '/api/v1/admin/invites', { method: 'POST', cookie: ownerCookie })).json();
 const token = new URLSearchParams(new URL(inv.url).hash.slice(1)).get('token');
 const member = await req(sourceEnv, '/api/v1/auth/register', { method: 'POST', data: { token, username: 'friend', display_name: 'Друг', password } });
 assert.equal(member.status, 201);
 const form = new FormData(); form.append('body', 'Публикация для проверки восстановления'); form.append('images', new Blob([await readFile(resolve(root, 'tests/fixtures/lake.png'))], { type: 'image/png' }), 'lake.png');
 const postResponse = await req(sourceEnv, '/api/v1/posts', { method: 'POST', cookie: ownerCookie, form }); assert.equal(postResponse.status, 201);
 const post = await postResponse.json();
 const original = Buffer.from(await (await req(sourceEnv, post.images[0].url, { cookie: ownerCookie })).arrayBuffer());
 // Same advisory lock as offline CLI: maintenance must refuse a live server.
 const locked = spawnSync(resolve(root, '.local/admin'), ['reconcile-media'], { env: sourceEnv, stdio: 'ignore' }); assert.notEqual(locked.status, 0);
 await stop(sourceServer); sourceServer = null;
 command('pg_dump', ['--format=custom', '--no-owner', '--no-acl', '--file', resolve(bundle, 'database.dump')], pgEnvFor(sourceName));
 command('tar', ['-czf', resolve(bundle, 'uploads.tar.gz'), '-C', sourceFiles, '.'], process.env);
 command('python3', [resolve(root, 'deploy/backup-format.py'), 'create', bundle, sourceName, 'native-rehearsal'], process.env);
 command('python3', [resolve(root, 'deploy/backup-format.py'), 'verify', bundle, restoredName, 'native-rehearsal'], process.env);
 sourceServer = await start(sourceEnv);
 // Verify persistence on restart before testing the independent restore.
 assert.equal((await req(sourceEnv, '/api/v1/posts/' + post.id, { cookie: ownerCookie })).status, 200);
 command('pg_restore', ['--no-owner', '--no-acl', '--exit-on-error', '--single-transaction', '--dbname', restoredName, resolve(bundle, 'database.dump')], pgEnvFor(restoredName));
 command('tar', ['-xzf', resolve(bundle, 'uploads.tar.gz'), '-C', restoredFiles], process.env);
 command(resolve(root, '.local/admin'), ['migrate'], restoredEnv);
 command(resolve(root, '.local/admin'), ['reconcile-media'], restoredEnv);
 restoredServer = await start(restoredEnv);
 const friendCookie = await login(restoredEnv, 'friend');
 const readPost = await req(restoredEnv, '/api/v1/posts/' + post.id, { cookie: friendCookie }); assert.equal(readPost.status, 200); assert.deepEqual(await readPost.json(), post);
 const readImage = await req(restoredEnv, post.images[0].url, { cookie: friendCookie }); assert.equal(readImage.status, 200); assert.equal(readImage.headers.get('cache-control'), 'no-store'); assert.deepEqual(Buffer.from(await readImage.arrayBuffer()), original);
 assert.equal((await req(restoredEnv, post.images[0].url)).status, 401);
 assert.equal((await req(restoredEnv, '/api/v1/posts/' + post.id, { cookie: friendCookie, method: 'DELETE' })).status, 403);
 const repeat = await req(restoredEnv, '/api/v1/auth/register', { method: 'POST', data: { token, username: 'another', display_name: 'Another', password } }); assert.equal(repeat.status, 400);
 console.log('PASS: real pg_dump/pg_restore, upload archive/checksums, restart persistence, two accounts, invitation state, image bytes, guest/author permissions, maintenance lock');
} finally {
 await stop(sourceServer); await stop(restoredServer);
 for (const name of created.reverse()) await admin.query(`DROP DATABASE ${name}`);
 await admin.end(); await rm(temp, { recursive: true, force: true });
}
