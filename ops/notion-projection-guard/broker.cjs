#!/usr/bin/env node
'use strict';
// The broker owns planning and the durable page barrier. It has no Notion token.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const readline = require('node:readline');

class GuardError extends Error {
  constructor(code, message) { super(message); this.code = code; }
}
function fail(code, message) { throw new GuardError(code, message); }
const hash = value => crypto.createHash('sha256').update(value, 'utf8').digest('hex');
const clone = value => JSON.parse(JSON.stringify(value));
function id(value) {
  if (typeof value !== 'string' || !/^[a-f0-9]{32}$/i.test(value.replaceAll('-', '')))
    fail('PAGE_ID', 'A native page UUID is required');
  return value.replaceAll('-', '').toLowerCase();
}
function unwrap(value) {
  for (let n = 0; n < 5 && value?.structuredContent; n++) {
    if (value.isError) fail('FETCH_ERROR', 'Connector returned an error');
    value = value.structuredContent;
  }
  if (!value || value.isError || value.error) fail('FETCH_ERROR', 'No successful page result');
  return value;
}
function pageData(raw, expectedId) {
  const page = unwrap(raw);
  if (id(page.id) !== id(expectedId)) fail('WRONG_PAGE', 'Fetched page does not match the plan');
  if (page.truncated === true || (page.unknown_block_count ?? 0) !== 0 ||
      (page.unknown_block_ids?.length ?? 0) !== 0) fail('INCOMPLETE_READ', 'Incomplete native read');
  const text = page.text;
  if (typeof text !== 'string') fail('INCOMPLETE_READ', 'Missing page text');
  const begin = '<content>\n', end = '\n</content>';
  if (text.split(begin).length !== 2 || text.split(end).length !== 2)
    fail('INCOMPLETE_READ', 'Expected one complete content envelope');
  const start = text.indexOf(begin) + begin.length, stop = text.indexOf(end, start);
  if (stop < start || /<unknown\b|content (?:has been |is )?truncated/i.test(text))
    fail('INCOMPLETE_READ', 'Unknown or truncated blocks');
  const properties = text.match(/<properties>\n([\s\S]*?)\n<\/properties>/);
  if (!properties) fail('INCOMPLETE_READ', 'Missing properties envelope');
  // Compare parsed properties: key ordering is not a semantic change.
  let props;
  try { props = JSON.parse(properties[1]); } catch { fail('INCOMPLETE_READ', 'Invalid properties'); }
  const ordered = Object.fromEntries(Object.entries(props).sort(([a], [b]) => a.localeCompare(b)));
  return { body: text.slice(start, stop), properties: JSON.stringify(ordered),
    revision: page.page_last_edited_at ?? null, raw: clone(raw) };
}
function headings(body) {
  const result = []; let offset = 0, fence = null; const stack = [];
  const native = 'details|callout|columns|column|tabs|tab|table|synced_block|synced_block_reference|meeting-notes';
  const open = new RegExp('^\\s*<(' + native + ')(?:\\s|>)');
  const close = new RegExp('^\\s*</(' + native + ')>');
  for (const line of body.split('\n')) {
    const code = line.match(/^\s*(`{3,}|~{3,})/);
    if (code) {
      if (!fence) fence = { char: code[1][0], size: code[1].length };
      else if (code[1][0] === fence.char && code[1].length >= fence.size) fence = null;
    } else if (!fence) {
      const c = line.match(close), o = line.match(open);
      if (c) { if (stack.pop() !== c[1]) fail('STRUCTURE', 'Unbalanced native container'); }
      else if (o && !/\/>\s*$/.test(line)) stack.push(o[1]);
      else if (!stack.length) {
        const h = line.match(/^(#{1,4}) (.+)$/);
        if (h) result.push({ text: line, level: h[1].length, start: offset, end: offset + line.length });
      }
    }
    offset += line.length + 1;
  }
  if (fence || stack.length) fail('STRUCTURE', 'Incomplete fenced/native container');
  return result;
}
function validateEdit(edit, policy) {
  if (!edit || Object.keys(edit).some(k => !['pageId', 'heading', 'oldStr', 'newStr'].includes(k)))
    fail('EDIT_SCHEMA', 'Only pageId, heading, oldStr and newStr are accepted');
  id(edit.pageId);
  if (!policy.headingPrefixes.some(prefix => edit.heading?.startsWith(prefix)))
    fail('OWNERSHIP', 'Only explicit central-owned observation headings are editable');
  for (const field of ['oldStr', 'newStr']) {
    const value = edit[field];
    if (typeof value !== 'string' || !value.length || value.length > 1024 || /[\r\n\u0000]/.test(value))
      fail('RANGE_LIMIT', 'Use one nonempty exact literal of at most 1024 characters');
    if (/^\s*#|```|~~~|<\/?[a-z][^>]*>|QUEUE_EVENT_V1/i.test(value))
      fail('PROTECTED_SYNTAX', 'Headings, native blocks and own event records are protected');
  }
  if (edit.oldStr === edit.newStr) fail('NO_CHANGE', 'No-op is not an update');
}
function plan(page, edit) {
  const matches = headings(page.body).filter(h => h.text === edit.heading);
  if (matches.length !== 1) fail('AMBIGUOUS_SECTION', 'Central heading must occur exactly once');
  const section = matches[0];
  const boundary = headings(page.body).find(h => h.start > section.start && h.level <= section.level);
  if (!boundary) fail('UNBOUNDED_SECTION', 'A following peer/parent heading is required');
  const index = page.body.indexOf(edit.oldStr);
  if (index < 0 || page.body.indexOf(edit.oldStr, index + 1) >= 0)
    fail('AMBIGUOUS_LITERAL', 'Old literal must occur exactly once in the entire body');
  if (index <= section.end || index + edit.oldStr.length > boundary.start)
    fail('OUTSIDE_SECTION', 'Literal crosses the explicit central section');
  // A nested native/fenced/worker proof section is never central-owned.
  const content = page.body.slice(section.end, boundary.start);
  if (headings(page.body).some(h => h.start > section.end && h.start < boundary.start))
    fail('PROTECTED_SECTION', 'Nested headings require a separately reviewed central section');
  if (/```|~~~|<\/?(?:synced_block|synced_block_reference|page|database|folder|unknown|meeting-notes)\b|QUEUE_EVENT_V1/.test(content))
    fail('PROTECTED_SECTION', 'Native/fenced/worker content prevents editing this section');
  if (/\b(?:event_id|session_id|lease_state)\s*[":=]/.test(content))
    fail('PROTECTED_SECTION', 'Worker own proof is present in the section');
  const before = page.body.slice(0, index), after = page.body.slice(index + edit.oldStr.length);
  const expected = before + edit.newStr + after;
  if (page.body.includes(edit.newStr)) fail('AMBIGUOUS_RESULT', 'New literal already occurs in the body');
  return { expected, beforeHash: hash(before), afterHash: hash(after), index,
    request: { page_id: id(edit.pageId), command: 'update_content', allow_async: false,
      allow_deleting_content: false, content_updates: [
        { old_str: edit.oldStr, new_str: edit.newStr, replace_all_matches: false }] } };
}
function syncDir(dir) {
  const fd = fs.openSync(dir, 'r'); try { fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
}
function atomic(file, value) {
  const tmp = file + '.' + crypto.randomUUID() + '.tmp';
  const data = JSON.stringify(value);
  const fd = fs.openSync(tmp, 'wx', 0o600);
  try { fs.writeFileSync(fd, data); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
  try { fs.renameSync(tmp, file); syncDir(path.dirname(file)); }
  finally { if (fs.existsSync(tmp)) fs.unlinkSync(tmp); }
  if (fs.readFileSync(file, 'utf8') !== data) fail('PERSISTENCE', 'Journal readback differs');
}
function read(file) {
  const st = fs.lstatSync(file);
  if (!st.isFile() || st.isSymbolicLink() || (st.mode & 0o077)) fail('PERSISTENCE', 'Private regular file required');
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

class Broker {
  constructor(root, policy) {
    const centralPrefixes = ['## CURRENT CENTRAL OBSERVATION', '## CURRENT CENTRAL ACTION', '## CURRENT INCIDENT MAINTENANCE'];
    if (!policy || typeof policy.productionEnabled !== 'boolean' ||
        !Array.isArray(policy.productionPages) || !Array.isArray(policy.canaryPages) ||
        !Array.isArray(policy.headingPrefixes) || !policy.headingPrefixes.length ||
        policy.headingPrefixes.some(p => !centralPrefixes.includes(p))) fail('POLICY', 'Invalid central-owned policy');
    for (const page of [...policy.productionPages, ...policy.canaryPages]) {
      if (id(page) !== page) fail('POLICY', 'Policy requires canonical page UUIDs');
    }
    this.root = path.resolve(root); this.policy = policy; this.file = path.join(this.root, 'journal.json');
    fs.mkdirSync(this.root, { recursive: true, mode: 0o700 });
    const st = fs.lstatSync(this.root);
    if (!st.isDirectory() || st.isSymbolicLink() || (st.mode & 0o077)) fail('PERSISTENCE', 'Private state directory required');
    // One process owns all batches. A process crash leaves this lock fail-closed.
    this.lock = path.join(this.root, 'broker.lock');
    try { fs.writeFileSync(this.lock, JSON.stringify({ pid: process.pid, at: new Date().toISOString() }), { flag: 'wx', mode: 0o600 }); }
    catch { fail('LOCKED', 'Existing broker lock requires owner inspection'); }
    syncDir(this.root);
    this.state = fs.existsSync(this.file) ? read(this.file) : { version: 1, batches: {}, active: null };
  }
  close() { fs.unlinkSync(this.lock); syncDir(this.root); }
  save() { atomic(this.file, this.state); }
  current(batchId) {
    const b = this.state.batches[batchId];
    if (!b || this.state.active !== batchId) fail('BATCH', 'Batch is not current');
    if (b.status === 'HALTED') fail('HALTED', 'Manual reconciliation required; no later page is authorized');
    return b;
  }
  snapshot(batchId, n) { return path.join(this.root, hash(batchId) + '-' + n + '.json'); }
  handle(input) {
    const { action, batchId } = input;
    if (action === 'status') return clone(this.state);
    if (action === 'close') return { status: 'CLOSED' };
    if (typeof batchId !== 'string' || !/^[a-zA-Z0-9_.:-]{1,150}$/.test(batchId) ||
        ['__proto__', 'prototype', 'constructor'].includes(batchId)) fail('BATCH_ID', 'Stable safe observation ID required');
    if (action === 'begin') {
      const edits = input.edits;
      if (!Array.isArray(edits) || !edits.length || edits.length > 30) fail('EDIT_SCHEMA', '1–30 page edits required');
      const seen = new Set();
      for (const edit of edits) {
        validateEdit(edit, this.policy);
        const page = id(edit.pageId);
        if (seen.has(page)) fail('DUPLICATE_PAGE', 'One minimal literal per page per batch');
        seen.add(page);
        const isCanary = this.policy.canaryPages.includes(page);
        if (!isCanary && (!this.policy.productionEnabled || !this.policy.productionPages.includes(page)))
          fail('PRODUCTION_HOLD', 'Production remains blocked until incident restoration');
      }
      const key = hash(JSON.stringify(edits));
      const previous = Object.hasOwn(this.state.batches,batchId) ? this.state.batches[batchId] : null;
      if (previous) {
        if (previous.planHash !== key) fail('BATCH_REUSE', 'Observation ID cannot bind different edits');
        return { status: previous.status, next: previous.next, reused: true };
      }
      if (this.state.active) fail('UNRESOLVED_BATCH', 'Previous batch must verify or be reconciled before any new batch');
      this.state.batches[batchId] = { planHash: key, edits: clone(edits), next: 0, status: 'READY', receipt: null };
      this.state.active = batchId; this.save(); return { status: 'READY', next: 0 };
    }
    const b = this.current(batchId), edit = b.edits[b.next];
    if (action === 'halt') {
      b.status = 'HALTED'; b.error = String(input.reason ?? 'Connector or runner failed').slice(0,1000);
      this.save(); return { status: 'HALTED', next: b.next };
    }
    if (action === 'prepare') {
      if (b.status !== 'READY') fail('PAGE_BARRIER', 'Prior page has not verified');
      const page = pageData(input.page, edit.pageId), p = plan(page, edit);
      const ticket = crypto.randomUUID();
      const snapshot = { version: 1, batchId, index: b.next, ticket, edit, page,
        expected: p.expected, beforeHash: p.beforeHash, afterHash: p.afterHash,
        bodyHash: hash(page.body), expectedHash: hash(p.expected), propertiesHash: hash(page.properties) };
      const file = this.snapshot(batchId, b.next);
      // Snapshot is durably saved and re-read before the write ticket exists.
      atomic(file, snapshot);
      const reread = read(file);
      if (hash(reread.page.body) !== snapshot.bodyHash || hash(reread.expected) !== snapshot.expectedHash)
        fail('PERSISTENCE', 'Snapshot digest mismatch');
      b.status = 'PREPARED'; b.receipt = { ticket, snapshot: file, snapshotHash: hash(JSON.stringify(snapshot)) };
      this.save(); return { status: 'PREPARED', ticket, bodyHash: snapshot.bodyHash, snapshot: file };
    }
    if (!b.receipt || input.ticket !== b.receipt.ticket) fail('TICKET', 'Ticket does not match the current page');
    const snapshot = read(b.receipt.snapshot);
    if (hash(JSON.stringify(snapshot)) !== b.receipt.snapshotHash) fail('PERSISTENCE', 'Immutable snapshot changed');
    if (action === 'authorize') {
      if (b.status !== 'PREPARED') fail('PAGE_BARRIER', 'Write has already been authorized; never retry it');
      const page = pageData(input.page, edit.pageId);
      if (page.body !== snapshot.page.body || page.properties !== snapshot.page.properties ||
          page.revision !== snapshot.page.revision) fail('CONCURRENT_CHANGE', 'Page changed since the saved preimage');
      const p = plan(page, edit);
      if (p.expected !== snapshot.expected) fail('PLAN_DRIFT', 'Recomputed plan differs');
      b.status = 'WRITE_PENDING'; this.save();
      return { status: 'WRITE_PENDING', request: p.request, ticket: b.receipt.ticket };
    }
    if (action === 'verify') {
      if (b.status !== 'WRITE_PENDING') fail('PAGE_BARRIER', 'No pending write to verify');
      const page = pageData(input.page, edit.pageId);
      if (page.body !== snapshot.expected || page.properties !== snapshot.page.properties) {
        b.status = 'HALTED'; b.error = 'READBACK_MISMATCH';
        const actualFile = this.snapshot(batchId, b.next) + '.actual.json';
        // Set the barrier even if saving the unexpected readback fails.
        this.save(); atomic(actualFile, { page, actualHash: hash(page.body), at: new Date().toISOString() });
        fail('READBACK_MISMATCH', 'Protected content or properties changed; batch is halted');
      }
      const n = snapshot.edit.oldStr.length, m = snapshot.edit.newStr.length;
      const index = snapshot.page.body.indexOf(snapshot.edit.oldStr);
      if (hash(page.body.slice(0,index)) !== snapshot.beforeHash ||
          hash(page.body.slice(index+m)) !== snapshot.afterHash || page.body.length !== snapshot.page.body.length-n+m)
        fail('PROTECTED_CHANGE', 'Protected prefix/suffix differs');
      atomic(b.receipt.snapshot + '.verified.json', { page, bodyHash: hash(page.body), at: new Date().toISOString() });
      b.next++; b.receipt = null; b.status = b.next === b.edits.length ? 'COMPLETED' : 'READY';
      if (b.status === 'COMPLETED') this.state.active = null;
      this.save(); return { status: b.status, next: b.next, protectedContent: 'UNCHANGED', bodyHash: hash(page.body) };
    }
    fail('ACTION', 'Unknown broker action');
  }
}

if (require.main === module) {
  const args = process.argv.slice(2);
  const state = args[args.indexOf('--state')+1], policyFile = args[args.indexOf('--policy')+1];
  if (!args.includes('--stdio') || !args.includes('--state') || !args.includes('--policy')) {
    process.stderr.write('Usage: broker.cjs --stdio --state ABSOLUTE_DIR --policy ABSOLUTE_JSON\n'); process.exit(2);
  }
  let broker;
  try { broker = new Broker(state, JSON.parse(fs.readFileSync(policyFile, 'utf8'))); }
  catch (e) { process.stdout.write('NPG/1 '+JSON.stringify({ code: e.code ?? 'ERROR', error: e.message })+'\n'); process.exit(1); }
  process.stdout.write('NPG/1 {"ready":true}\n');
  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
  rl.on('line', line => {
    let request;
    try {
      request = JSON.parse(line);
      const result = broker.handle(request);
      process.stdout.write('NPG/1 '+JSON.stringify({ requestId: request.requestId, ok: true, result })+'\n', () => {
        if (request.action === 'close') { rl.close(); process.stdin.destroy(); process.exit(0); }
      });
    } catch (e) {
      process.stdout.write('NPG/1 '+JSON.stringify({ requestId: request?.requestId, ok: false, code: e.code ?? 'ERROR', error: e.message })+'\n');
    }
  });
  let closed = false;
  function close() { if (!closed) { closed = true; broker.close(); } }
  rl.on('close', close); process.on('exit', close);
  for (const signal of ['SIGTERM','SIGINT','SIGHUP']) process.on(signal, () => { close(); process.exit(0); });
}
module.exports = { Broker, GuardError, hash, pageData, plan, validateEdit, atomic };
