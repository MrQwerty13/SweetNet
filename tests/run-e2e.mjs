// Every run gets a fresh schema and upload directory; shared DB tables are untouched.
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import pg from 'pg';
const root = resolve(import.meta.dirname, '..');
const databaseURL = process.env.TEST_DATABASE_URL;
if (!databaseURL) throw new Error('TEST_DATABASE_URL required; database name must end _test');
const db = new pg.Client({ connectionString: databaseURL });
await db.connect();
const { rows } = await db.query('SELECT current_database() AS name');
if (!rows[0].name.endsWith('_test')) throw new Error('Refusing non-test database');
const schema = `e2e_${randomUUID().replaceAll('-', '')}`;
const uploads = await mkdtemp(resolve(tmpdir(), 'sweetnet-e2e-'));
const url = new URL(databaseURL); url.searchParams.set('search_path', schema);
const port = process.env.E2E_PORT || '18080';
const origin = `http://127.0.0.1:${port}`;
const password = randomBytes(24).toString('hex');
const env = { ...process.env, DATABASE_URL: url.href, APP_ENV: 'development', APP_ORIGIN: origin, HTTP_ADDR: `127.0.0.1:${port}`, UPLOAD_DIR: uploads, WEB_DIR: resolve(root, 'web/dist'), E2E_BASE_URL: origin, E2E_USERNAME: 'owner', E2E_PASSWORD: password, E2E_DATABASE_URL: url.href };
let server;
function admin(args, input) { const r = spawnSync(resolve(root, '.local/admin'), args, { env, input, encoding: 'utf8' }); if (r.status !== 0) throw new Error(`admin ${args[0]} failed: ${r.stderr}`); }
try {
 await db.query(`CREATE SCHEMA ${schema}`);
 admin(['migrate']);
 admin(['bootstrap', '--username', 'owner', '--display-name', 'Михаил'], password + '\n');
 server = spawn(resolve(root, '.local/server'), [], { env, stdio: ['ignore', 'pipe', 'pipe'] });
 server.stderr.on('data', chunk => process.stderr.write(chunk));
 let ready = false;
 for (let i = 0; i < 100; i++) {
  if (server.exitCode !== null) throw new Error('server exited before readiness');
  try { if ((await fetch(`${origin}/readyz`)).ok) { ready = true; break; } } catch {}
  await new Promise(r => setTimeout(r, 100));
 }
 if (!ready) throw new Error('server readiness timeout');
 const child = spawn(process.execPath, [resolve(root, 'tests/node_modules/@playwright/test/cli.js'), 'test', ...process.argv.slice(2)], { cwd: resolve(root, 'tests'), env, stdio: 'inherit' });
 const exit = await new Promise(r => child.once('exit', r));
 process.exitCode = exit ?? 1;
} finally {
 if (server && server.exitCode === null) { server.kill('SIGTERM'); await new Promise(r => server.once('exit', r)); }
 await db.query(`DROP SCHEMA IF EXISTS ${schema} CASCADE`);
 await db.end();
 await rm(uploads, { recursive: true, force: true });
}
