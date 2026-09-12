// Full container rehearsal. Only creates random, isolated test projects.
// Requires a working Docker daemon. No external publication.
import { spawnSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
const root = resolve(import.meta.dirname, '..');
const suffix = randomUUID().replaceAll('-', '').slice(0, 12);
const source = `sweetnet-test-${suffix}`;
const target = `sweetnet-restore-test-${suffix}`;
const image = `sweetnet-test:${suffix}`;
const temp = await mkdtemp(resolve(tmpdir(), 'sweetnet-compose-'));
const sourceEnv = resolve(temp, 'source.env'); const targetEnv = resolve(temp, 'target.env');
const bundle = resolve(temp, 'backup');
const sourcePort = process.env.COMPOSE_TEST_PORT || '18880';
const targetPort = process.env.COMPOSE_RESTORE_PORT || '18881';
const password = randomBytes(24).toString('hex');
const owned = [];
function run(file, args, options = {}) {
 const result = spawnSync(file, args, { cwd: root, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024, ...options });
 if (result.status !== 0) throw new Error(`${file} ${args[0]} failed (private command output suppressed)`);
 return result.stdout;
}
function compose(env, project, args, options) { return run('docker', ['compose', '--env-file', env, '-f', resolve(root, 'compose.yaml'), '-p', project, ...args], options); }
async function settings(file, port) {
 await writeFile(file, `APP_ENV=development\nAPP_ORIGIN=http://127.0.0.1:${port}\nAPP_PORT=${port}\nPOSTGRES_USER=sweetnet\nPOSTGRES_DB=sweetnet\nPOSTGRES_PASSWORD=${randomBytes(32).toString('hex')}\nSWEETNET_IMAGE=${image}\n`, { mode: 0o600 });
}
async function req(port, path, { method = 'GET', data, cookie, form } = {}) {
 const headers = { Origin: `http://127.0.0.1:${port}` };
 if (cookie) headers.Cookie = cookie;
 if (data) headers['Content-Type'] = 'application/json';
 return fetch(`http://127.0.0.1:${port}${path}`, { method, headers, body: form ?? (data ? JSON.stringify(data) : undefined) });
}
async function login(port) { const r = await req(port, '/api/v1/auth/login', { method: 'POST', data: { username: 'owner', password } }); assert.equal(r.status, 200); return r.headers.getSetCookie()[0].split(';')[0]; }
try {
 run('docker', ['info']);
 await settings(sourceEnv, sourcePort); await settings(targetEnv, targetPort);
 // Refuse an existing namespace even though names are random.
 for (const project of [source, target]) {
  assert.equal(run('docker', ['ps', '-aq', '--filter', `label=com.docker.compose.project=${project}`]).trim(), '');
  assert.equal(run('docker', ['volume', 'ls', '-q', '--filter', `label=com.docker.compose.project=${project}`]).trim(), '');
 }
 owned.push([sourceEnv, source]);
 console.log('Building and starting isolated Docker test project');
 compose(sourceEnv, source, ['up', '-d', '--build', '--wait', '--wait-timeout', '180']);
 compose(sourceEnv, source, ['exec', '-T', 'app', '/app/admin', 'bootstrap', '--username', 'owner', '--display-name', 'Test owner'], { input: password + '\n' });
 const cookie = await login(sourcePort);
 const form = new FormData(); form.append('body', 'Docker restore verification'); form.append('images', new Blob([await readFile(resolve(root, 'tests/fixtures/lake.png'))], { type: 'image/png' }), 'lake.png');
 const create = await req(sourcePort, '/api/v1/posts', { method: 'POST', form, cookie }); assert.equal(create.status, 201); const post = await create.json();
 const photo = Buffer.from(await (await req(sourcePort, post.images[0].url, { cookie })).arrayBuffer());
 compose(sourceEnv, source, ['restart', 'app']);
 for (let i = 0; i < 100; i++) { try { if ((await req(sourcePort, '/readyz')).ok) break; } catch {} await new Promise(r => setTimeout(r, 100)); }
 assert.equal((await req(sourcePort, '/api/v1/posts/' + post.id, { cookie })).status, 200);
 run('bash', ['scripts/backup.sh', sourceEnv, source, bundle]);
 assert.equal((await req(sourcePort, '/readyz')).status, 200);
 owned.push([targetEnv, target]);
 run('bash', ['scripts/restore.sh', targetEnv, target, bundle, '--new-environment']);
 const restoredCookie = await login(targetPort);
 const restored = await req(targetPort, '/api/v1/posts/' + post.id, { cookie: restoredCookie }); assert.equal(restored.status, 200); assert.deepEqual(await restored.json(), post);
 const restoredPhoto = await req(targetPort, post.images[0].url, { cookie: restoredCookie }); assert.equal(restoredPhoto.status, 200); assert.deepEqual(Buffer.from(await restoredPhoto.arrayBuffer()), photo);
 assert.equal((await req(targetPort, post.images[0].url)).status, 401);
 assert.equal((await req(targetPort, '/api/v1/posts')).status, 401);
 console.log('PASS: Docker build, migration ordering, persistence, backup script, restore script, protected media');
} finally {
 for (const [env, project] of owned.reverse()) {
  // These namespaces were verified absent and created solely by this test.
  spawnSync('docker', ['compose', '--env-file', env, '-f', resolve(root, 'compose.yaml'), '-p', project, 'down', '--volumes', '--remove-orphans'], { cwd: root, stdio: 'ignore' });
 }
 await rm(temp, { recursive: true, force: true });
}
