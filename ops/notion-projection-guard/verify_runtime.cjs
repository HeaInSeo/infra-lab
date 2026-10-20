'use strict';
// Offline and read-only: never import the broker, take its lock or open its journal.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const runtimeNames = ['broker.cjs', 'runner.js', 'policy.json'];
const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
function requireThat(ok, code) {
  if (!ok) throw Object.assign(new Error(code), { code });
}
function read(file) {
  requireThat(fs.lstatSync(file).isFile(), 'REGULAR_FILE_REQUIRED');
  return fs.readFileSync(file);
}
function verifyRuntime({ release, runtimeDir, receiptPath, sourceDir }) {
  requireThat(release?.version === 1 && release.repository === 'HeaInSeo/infra-lab'
    && /^[a-f0-9]{40}$/.test(release.sourceCommit), 'RELEASE_SCHEMA');
  requireThat(Object.keys(release.runtimeSha256 ?? {}).sort().join(',') === 'broker.cjs,runner.js'
    && Object.values(release.runtimeSha256).every(h => /^[a-f0-9]{64}$/.test(h))
    && /^[a-f0-9]{64}$/.test(release.policySha256), 'RELEASE_SCHEMA');
  requireThat(path.isAbsolute(runtimeDir) && path.isAbsolute(receiptPath)
    && (sourceDir === undefined || path.isAbsolute(sourceDir)), 'ABSOLUTE_PATH_REQUIRED');
  const expected = { ...release.runtimeSha256, 'policy.json': release.policySha256 };
  const files = [...runtimeNames, 'approved.sha256'].map(n => path.join(runtimeDir, n));
  files.push(receiptPath);
  const before = files.map(read);
  const lines = before[3].toString('utf8').split('\n');
  if (lines.at(-1) === '') lines.pop();
  requireThat(lines.length === 3, 'APPROVED_MANIFEST');
  const manifest = {};
  for (const line of lines) {
    const match = /^([a-f0-9]{64})  (broker\.cjs|runner\.js|policy\.json)$/.exec(line);
    requireThat(match && !Object.hasOwn(manifest, match[2]), 'APPROVED_MANIFEST');
    manifest[match[2]] = match[1];
  }
  const observed = {};
  for (let i = 0; i < runtimeNames.length; i++) {
    const name = runtimeNames[i];
    observed[name] = sha256(before[i]);
    requireThat(observed[name] === expected[name], 'RUNTIME_HASH_MISMATCH');
    requireThat(manifest[name] === expected[name], 'APPROVED_MANIFEST');
  }
  const receipt = JSON.parse(before[4].toString('utf8'));
  requireThat(receipt.sourceCommit === release.sourceCommit
    && typeof receipt.at === 'string' && Number.isFinite(Date.parse(receipt.at))
    && runtimeNames.every(n => receipt.after?.[n] === expected[n]), 'INSTALL_RECEIPT_MISMATCH');
  if (sourceDir !== undefined) {
    for (const name of ['broker.cjs', 'runner.js']) {
      requireThat(sha256(read(path.join(sourceDir, name))) === expected[name], 'SOURCE_HASH_MISMATCH');
    }
    requireThat(release.hostedEntryPoint?.path === 'ops/notion-projection-guard/complete_projection.js'
      && /^[a-f0-9]{64}$/.test(release.hostedEntryPoint.sha256), 'RELEASE_SCHEMA');
    requireThat(sha256(read(path.join(sourceDir, 'complete_projection.js')))
      === release.hostedEntryPoint.sha256, 'SOURCE_HASH_MISMATCH');
  }
  requireThat(files.every((file, i) => read(file).equals(before[i])), 'CHANGED_DURING_READ');
  return { status: 'MATCH', observedAt: new Date().toISOString(),
    sourceCommit: release.sourceCommit, installedAt: receipt.at, sha256: observed,
    sourceFilesChecked: sourceDir !== undefined };
}
module.exports = { verifyRuntime };
if (require.main === module) {
  try {
    const args = process.argv.slice(2), options = {};
    const names = { '--release': 'releasePath', '--runtime': 'runtimeDir',
      '--receipt': 'receiptPath', '--source': 'sourceDir' };
    for (let i = 0; i < args.length; i += 2) {
      const key = names[args[i]];
      requireThat(key && !Object.hasOwn(options, key) && args[i + 1]
        && !args[i + 1].startsWith('--'), 'ARGUMENTS');
      options[key] = args[i + 1];
    }
    requireThat(options.releasePath && options.runtimeDir && options.receiptPath, 'ARGUMENTS');
    const release = JSON.parse(read(options.releasePath).toString('utf8'));
    console.log(JSON.stringify(verifyRuntime({ ...options, release })));
  } catch (error) {
    console.log(JSON.stringify({ status: 'MISMATCH', code: error.code ?? 'INVALID_INPUT' }));
    process.exitCode = 1;
  }
}
