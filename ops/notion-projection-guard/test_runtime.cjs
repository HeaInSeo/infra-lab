'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { verifyRuntime } = require('./verify_runtime.cjs');
const hash = b => crypto.createHash('sha256').update(b).digest('hex');
function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'npg-version-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const sourceDir = path.join(dir, 'source'); fs.mkdirSync(sourceDir);
  const pins = {};
  for (const name of ['broker.cjs', 'runner.js', 'policy.json']) {
    // Executing/importing a runtime candidate would fail this test immediately.
    const bytes = 'throw new Error("must not execute runtime"); // ' + name;
    fs.writeFileSync(path.join(dir, name), bytes);
    pins[name] = hash(bytes);
    if (name !== 'policy.json') fs.writeFileSync(path.join(sourceDir, name), bytes);
  }
  fs.writeFileSync(path.join(sourceDir, 'complete_projection.js'), 'hosted entry');
  const release = { version: 1, repository: 'HeaInSeo/infra-lab', sourceCommit: 'a'.repeat(40),
    runtimeSha256: { 'broker.cjs': pins['broker.cjs'], 'runner.js': pins['runner.js'] },
    policySha256: pins['policy.json'], hostedEntryPoint: {
      path: 'ops/notion-projection-guard/complete_projection.js', sha256: hash('hosted entry') } };
  fs.writeFileSync(path.join(dir, 'approved.sha256'), Object.entries(pins).map(([n, h]) => h + '  ' + n + '\n').join(''));
  const receiptPath = path.join(dir, 'receipt.json');
  fs.writeFileSync(receiptPath, JSON.stringify({ sourceCommit: release.sourceCommit,
    at: '2026-10-19T23:02:49.009Z', after: pins }));
  const journalPath = path.join(dir, 'journal.json');
  fs.writeFileSync(journalPath, '{"active":"unrelated-live-batch"}');
  fs.writeFileSync(path.join(dir, 'broker.lock'), 'existing worker lock');
  return { release, runtimeDir: dir, receiptPath, sourceDir };
}
test('offline verification works beside a live lock and changes no file bytes or modes', t => {
  const f = fixture(t);
  const snapshot = () => Object.fromEntries(fs.readdirSync(f.runtimeDir, { withFileTypes: true })
    .filter(e => e.isFile()).map(e => { const p = path.join(f.runtimeDir, e.name);
      return [e.name, { hash: hash(fs.readFileSync(p)), mode: fs.statSync(p).mode }]; }));
  const before = snapshot();
  assert.equal(verifyRuntime(f).status, 'MATCH');
  assert.deepEqual(snapshot(), before);
});
for (const name of ['broker.cjs', 'runner.js', 'policy.json']) {
  test('rejects changed runtime even when approved.sha256 and receipt are changed to match: ' + name, t => {
    const f = fixture(t), file = path.join(f.runtimeDir, name);
    fs.writeFileSync(file, 'different runtime');
    const manifest = path.join(f.runtimeDir, 'approved.sha256');
    fs.writeFileSync(manifest, fs.readFileSync(manifest, 'utf8').replace(hash('throw new Error("must not execute runtime"); // ' + name), hash('different runtime')));
    const receipt = JSON.parse(fs.readFileSync(f.receiptPath)); receipt.after[name] = hash('different runtime');
    fs.writeFileSync(f.receiptPath, JSON.stringify(receipt));
    assert.throws(() => verifyRuntime(f), { code: 'RUNTIME_HASH_MISMATCH' });
  });
}
test('rejects duplicate manifest entries and stale install receipts', t => {
  const f = fixture(t), manifest = path.join(f.runtimeDir, 'approved.sha256');
  const original = fs.readFileSync(manifest, 'utf8');
  fs.writeFileSync(manifest, original.split('\n')[0] + '\n' + original.split('\n')[0] + '\n' + original.split('\n')[2] + '\n');
  assert.throws(() => verifyRuntime(f), { code: 'APPROVED_MANIFEST' });
  fs.writeFileSync(manifest, original);
  const receipt = JSON.parse(fs.readFileSync(f.receiptPath)); receipt.sourceCommit = 'b'.repeat(40);
  fs.writeFileSync(f.receiptPath, JSON.stringify(receipt));
  assert.throws(() => verifyRuntime(f), { code: 'INSTALL_RECEIPT_MISMATCH' });
});
test('rejects drift in the hosted entry source, independent of installed runtime', t => {
  const f = fixture(t); fs.writeFileSync(path.join(f.sourceDir, 'complete_projection.js'), 'other entry');
  assert.throws(() => verifyRuntime(f), { code: 'SOURCE_HASH_MISMATCH' });
});
test('CLI rejects duplicate flags and absent files with nonzero exit, without leaking file contents', t => {
  const f = fixture(t), script = path.join(__dirname, 'verify_runtime.cjs');
  const result = spawnSync(process.execPath, [script, '--release', f.receiptPath,
    '--release', f.receiptPath, '--runtime', f.runtimeDir, '--receipt', f.receiptPath], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.deepEqual(JSON.parse(result.stdout), { status: 'MISMATCH', code: 'ARGUMENTS' });
  const missing = spawnSync(process.execPath, [script, '--release', path.join(f.runtimeDir, 'absent.json'),
    '--runtime', f.runtimeDir, '--receipt', f.receiptPath], { encoding: 'utf8' });
  assert.equal(missing.status, 1);
  assert.deepEqual(JSON.parse(missing.stdout), { status: 'MISMATCH', code: 'ENOENT' });
});
