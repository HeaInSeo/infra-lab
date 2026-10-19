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
// Reviewed legacy central anchors only. A policy cannot authorize arbitrary
// CURRENT/worker headings or grant these pages the general prefix permission.
const legacyCentralSections = Object.freeze({
  '3f586e6dd1d181ebb43dd7de62aa2ca4': '## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3',
  '3f586e6dd1d181cd9f5bdcdab6290de7': '## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED'
});
function exactCentralSection(edit, policy) {
  const pageId = id(edit.pageId);
  return legacyCentralSections[pageId] === edit.heading &&
    (policy.centralSections ?? []).some(s => s.pageId === pageId && s.heading === edit.heading);
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
  const pageId = id(edit.pageId);
  const ownsHeading = Object.hasOwn(legacyCentralSections, pageId)
    ? exactCentralSection(edit, policy)
    : policy.headingPrefixes.some(prefix => edit.heading?.startsWith(prefix));
  if (!ownsHeading)
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
function validateScope(edit, policy) {
  validateEdit(edit, policy);
  const pageId = id(edit.pageId);
  if (!policy.canaryPages.includes(pageId) && (!policy.productionEnabled ||
      (!policy.productionPages.includes(pageId) && !exactCentralSection(edit, policy))))
    fail('PRODUCTION_HOLD', 'Production remains blocked until incident restoration');
}
function plainMarkup(text) {
  // Require explicit code/link markup for hostname-shaped filenames. Notion
  // can turn these into links, invalidating the exact post-write comparison.
  return text.replace(/(?<![\\`])(`{1,2})(?!`)([\s\S]*?)(?<!`)\1(?!`)/g, match => ' '.repeat(match.length))
    .replace(/(?<!\\)\[[^\]\\\r\n]+\]\(https?:\/\/[^\s()\\]+\)/g, match => ' '.repeat(match.length));
}
function bareAutolinks(text) {
  return [...plainMarkup(text).matchAll(/\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}\b/gi)]
    .map(match => match[0]);
}
function stableNewText(edit) {
  inlineSpans(edit.oldStr);inlineSpans(edit.newStr);
  if (bareAutolinks(edit.newStr).length)
    fail('AUTOLINK_RISK', 'Wrap hostname-shaped filenames and URLs in inline code or an explicit Markdown link');
  if (plainMarkup(edit.newStr).includes('->'))
    fail('MARKDOWN_ESCAPE_RISK', 'Use a Unicode arrow or inline code instead of a plain ASCII arrow');
}
function inlineSpans(text) {
  // Escapes outside spans are Markdown escapes; inside, backslashes are literal.
  const spans=[]; let open=null;
  for(let i=0;i<text.length;) {
    if(!open && text[i]==='\\') { i+=2; continue; }
    if(text[i]!=='`') { i++; continue; }
    let end=i+1; while(text[end]==='`')end++;
    const size=end-i;
    if(!open)open={start:i,size};
    else if(size===open.size){spans.push({start:open.start,end});open=null;}
    i=end;
  }
  if(open)fail('INLINE_CODE_BOUNDARY','Use complete balanced inline-code fields; never cut a field to fit the limit');
  return spans;
}
function inlineEdit(body,index,edit) {
  const start=body.lastIndexOf('\n',index-1)+1;
  const stop=body.indexOf('\n',index+edit.oldStr.length);
  const line=body.slice(start,stop<0?body.length:stop),a=index-start,b=a+edit.oldStr.length;
  for(const span of inlineSpans(line))
    if((a>span.start&&a<span.end)||(b>span.start&&b<span.end))
      fail('INLINE_CODE_BOUNDARY','Selection begins or ends inside an existing inline-code field');
  inlineSpans(edit.oldStr);inlineSpans(edit.newStr);
  inlineSpans(line.slice(0,a)+edit.newStr+line.slice(b));
}
function reviewedArrowLiteral(edit) {
  if (bareAutolinks(edit.newStr).length ||
      plainMarkup(edit.newStr).split(' -> ').length !== 3 || edit.newStr.split('->').length !== 3)
    fail('RECONCILE_SCOPE', 'Only the two observed plain central arrows are admitted');
  return edit.newStr.replaceAll(' -> ', ' -\\> ');
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
  inlineEdit(page.body,index,edit);
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
    if (policy.canaryPages.some(page => Object.hasOwn(legacyCentralSections, page)))
      fail('POLICY', 'Legacy central task pages are never canaries');
    const sections = Object.hasOwn(policy, 'centralSections') ? policy.centralSections : [];
    if (!Array.isArray(sections) || sections.length > 2)
      fail('POLICY', 'Only the two reviewed legacy central anchors are supported');
    const seen = new Set();
    for (const section of sections) {
      if (!section || Object.keys(section).length !== 2 ||
          typeof section.pageId !== 'string' || !Object.hasOwn(legacyCentralSections, section.pageId) ||
          section.heading !== legacyCentralSections[section.pageId] || seen.has(section.pageId))
        fail('POLICY', 'Exact reviewed page/heading pairs required');
      seen.add(section.pageId);
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
    this.recovery = null;
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
  reconcileAutolink(input) {
    return this.reconcileRendering(input, 'package');
  }
  reconcileRendering(input, mode) {
    if (!['package', 'arrows'].includes(mode)) fail('RECONCILE_SCOPE', 'Unknown rendering incident');
    const { batchId, planHash, ticket, snapshotHash, actualHash, actualSnapshotHash } = input;
    const b = this.state.batches[batchId];
    // These are separately invoked, pinned incident recoveries, never a
    // normalization rule in verify() and never a second Notion mutation.
    if (!b || this.state.active !== batchId || b.status !== 'HALTED' ||
        b.error !== 'READBACK_MISMATCH' || input.expectedError !== b.error ||
        b.next !== 0 || b.edits.length !== (mode === 'package' ? 1 : 6) || b.planHash !== planHash ||
        hash(JSON.stringify(b.edits)) !== planHash || !b.receipt || b.receipt.ticket !== ticket ||
        b.receipt.snapshotHash !== snapshotHash || b.receipt.snapshot !== this.snapshot(batchId, 0))
      fail('RECONCILE_IDENTITY', 'Exact halted incident, immutable plan/ticket and snapshot required');
    const edit = b.edits[0];
    const seen = new Set();
    for (const [index, planned] of b.edits.entries()) {
      validateScope(planned, this.policy);
      const pageId = id(planned.pageId);
      if (seen.has(pageId)) fail('RECONCILE_IDENTITY', 'Duplicate recovery page');
      seen.add(pageId);
      if (index > 0) stableNewText(planned);
    }
    const file = b.receipt.snapshot, snapshot = read(file), actual = read(file + '.actual.json');
    const preimage = pageData(snapshot.page.raw, edit.pageId);
    const savedActual = pageData(actual.page.raw, edit.pageId);
    if (hash(JSON.stringify(snapshot)) !== snapshotHash || snapshot.batchId !== batchId ||
        snapshot.index !== 0 || snapshot.ticket !== ticket ||
        JSON.stringify(snapshot.edit) !== JSON.stringify(edit) ||
        snapshot.bodyHash !== hash(snapshot.page.body) ||
        snapshot.propertiesHash !== hash(snapshot.page.properties) ||
        snapshot.expectedHash !== hash(snapshot.expected) ||
        preimage.body !== snapshot.page.body || preimage.properties !== snapshot.page.properties ||
        preimage.revision !== snapshot.page.revision ||
        plan(snapshot.page, edit).expected !== snapshot.expected ||
        hash(JSON.stringify(actual)) !== actualSnapshotHash ||
        actual.actualHash !== actualHash || hash(actual.page.body) !== actualHash ||
        savedActual.body !== actual.page.body || savedActual.properties !== actual.page.properties ||
        savedActual.revision !== actual.page.revision)
      fail('PERSISTENCE', 'Canonical preimage or mismatched readback changed');
    let renderedLiteral, kind;
    if (mode === 'arrows') {
      renderedLiteral = reviewedArrowLiteral(edit); kind = 'REVIEWED_CENTRAL_ARROW_ESCAPING';
    } else {
      const token = 'package-cli.sh', rendered = '[package-cli.sh](http://package-cli.sh)';
      const risks = bareAutolinks(edit.newStr);
      if (risks.length !== 1 || risks[0] !== token || edit.newStr.split(token).length !== 2)
        fail('RECONCILE_SCOPE', 'Only the independently reviewed package-cli.sh autolink is admitted');
      renderedLiteral = edit.newStr.replace(token, rendered); kind = 'REVIEWED_PACKAGE_CLI_AUTOLINK';
    }
    const index = snapshot.page.body.indexOf(edit.oldStr);
    const before = snapshot.page.body.slice(0, index);
    const after = snapshot.page.body.slice(index + edit.oldStr.length);
    const expected = before + renderedLiteral + after;
    if (hash(before) !== snapshot.beforeHash || hash(after) !== snapshot.afterHash ||
        actual.page.body !== expected || actual.page.properties !== snapshot.page.properties)
      fail('RECONCILE_SCOPE', 'Mismatch is not confined to the reviewed rendering difference');
    const fresh = pageData(input.page, edit.pageId);
    if (fresh.body !== actual.page.body || fresh.properties !== actual.page.properties ||
        fresh.revision !== actual.page.revision)
      fail('RECONCILE_CHANGED', 'Fresh complete page must still equal the saved mismatched readback');
    const at = new Date().toISOString();
    const evidence = { kind, at, planHash, snapshotHash, actualHash, actualSnapshotHash,
      ticket, plannedExpectedHash:snapshot.expectedHash, protectedContent:'UNCHANGED', notionWrites:0 };
    atomic(file + '.verified.json', { page:fresh, bodyHash:hash(fresh.body), at, reconciliation:evidence });
    b.reconciliations = [...(b.reconciliations ?? []), evidence];
    b.next = 1; b.receipt = null; b.status = b.edits.length === 1 ? 'COMPLETED' : 'READY';
    if (b.status === 'COMPLETED') this.state.active = null;
    this.save();
    return { status:b.status, next:1, protectedContent:'UNCHANGED', bodyHash:hash(fresh.body),
      reconciliation:evidence };
  }
  verifiedExpected(snapshot, verified, batch, index) {
    const audit = verified.reconciliation;
    if (!audit) return snapshot.expected;
    if (index !== 0 || batch.edits.length !== 6 || audit.kind !== 'REVIEWED_CENTRAL_ARROW_ESCAPING' ||
        !(batch.reconciliations ?? []).some(record => JSON.stringify(record) === JSON.stringify(audit)) ||
        audit.planHash !== batch.planHash || audit.ticket !== snapshot.ticket ||
        audit.snapshotHash !== hash(JSON.stringify(snapshot)) || audit.plannedExpectedHash !== snapshot.expectedHash ||
        audit.protectedContent !== 'UNCHANGED' || audit.notionWrites !== 0 || verified.at !== audit.at)
      fail('PERSISTENCE', 'Reviewed rendering receipt is not bound to the original audit');
    const actual = read(this.snapshot(snapshot.batchId, index) + '.actual.json');
    const saved = pageData(actual.page.raw, snapshot.edit.pageId);
    const expected = plan(snapshot.page, { ...snapshot.edit, newStr:reviewedArrowLiteral(snapshot.edit) }).expected;
    if (hash(JSON.stringify(actual)) !== audit.actualSnapshotHash || actual.actualHash !== audit.actualHash ||
        hash(actual.page.body) !== audit.actualHash || actual.page.body !== expected ||
        actual.page.properties !== snapshot.page.properties || saved.body !== actual.page.body ||
        saved.properties !== actual.page.properties || saved.revision !== actual.page.revision)
      fail('PERSISTENCE', 'Reviewed rendering evidence changed');
    return expected;
  }
  reconcile(input) {
    const { batchId, planHash, expectedError, action } = input;
    const b = this.state.batches[batchId];
    if (this.state.active !== batchId || !b || b.status !== 'HALTED' ||
        b.planHash !== planHash || hash(JSON.stringify(b.edits)) !== planHash ||
        b.error !== expectedError || !b.receipt)
      fail('RECONCILE_IDENTITY', 'Exact halted batch, immutable plan and error required');
    // This narrowly admits the observed DC failure while awaiting authorize.
    // A Notion/async/readback failure or an unknown lost write cannot retry.
    if (!b.error.startsWith('REMOTE_ERROR: Error interacting with process: Invalid regular expression: ') ||
        !b.error.includes('"action":"authorize"') || !b.error.includes(JSON.stringify(batchId)) ||
        !b.error.includes(JSON.stringify(b.receipt.ticket)))
      fail('RECONCILE_SCOPE', 'Only an unwritten DC authorize prompt failure can be reconciled');
    for (const edit of b.edits) {
      validateScope(edit, this.policy);
    }
    const checkSnapshot = (snapshot, index) => {
      if (snapshot.batchId !== batchId || snapshot.index !== index ||
          JSON.stringify(snapshot.edit) !== JSON.stringify(b.edits[index]) ||
          hash(snapshot.page.body) !== snapshot.bodyHash ||
          hash(snapshot.page.properties) !== snapshot.propertiesHash ||
          hash(snapshot.expected) !== snapshot.expectedHash ||
          plan(snapshot.page, snapshot.edit).expected !== snapshot.expected)
        fail('PERSISTENCE', 'Canonical snapshot changed');
    };
    if (action === 'reconcile_receipts') {
      // Completed pages already have full protected-content verification.
      // Revalidate that immutable evidence; never write them again or restore
      // an old page over a worker's later legitimate records.
      const receipts = [];
      this.recovery = null;
      for (let index=0; index<b.next; index++) {
        const file=this.snapshot(batchId,index);
        const snapshot=read(file), verified=read(file+'.verified.json');
        checkSnapshot(snapshot,index);
        const preimage=pageData(snapshot.page.raw,b.edits[index].pageId);
        const readback=pageData(verified.page.raw,b.edits[index].pageId);
        const expected=this.verifiedExpected(snapshot,verified,b,index);
        if (preimage.body!==snapshot.page.body || preimage.properties!==snapshot.page.properties ||
            preimage.revision!==snapshot.page.revision ||
            verified.bodyHash!==hash(expected) || verified.page.body!==expected ||
            verified.page.properties!==snapshot.page.properties ||
            readback.body!==verified.page.body || readback.properties!==verified.page.properties ||
            readback.revision!==verified.page.revision)
          fail('PERSISTENCE','Completed protected-content receipt changed');
        receipts.push({ index, pageId:b.edits[index].pageId,
          snapshotHash:hash(JSON.stringify(snapshot)), verifiedHash:hash(JSON.stringify(verified)),
          bodyHash:verified.bodyHash, verifiedAt:verified.at });
      }
      this.recovery={ batchId,planHash,expectedError,next:b.next,receipts,mode:'DURABLE_VERIFIED_RECEIPTS' };
      return { status:'HALTED',next:b.next,priorVerified:b.next,receipts };
    }
    if (action === 'reconcile_verified') {
      const index = input.index;
      if (!Number.isInteger(index) || index < 0 || index >= b.next)
        fail('RECONCILE_ORDER', 'Only prior verified pages can be checked');
      if (index === 0) this.recovery = { batchId, planHash, expectedError, next:0 };
      const recovery = this.recovery;
      if (!recovery || recovery.batchId !== batchId || recovery.planHash !== planHash ||
          recovery.expectedError !== expectedError || recovery.next !== index)
        fail('RECONCILE_ORDER', 'Check every prior receipt in canonical order');
      const file = this.snapshot(batchId, index);
      const snapshot = read(file), verified = read(file + '.verified.json');
      checkSnapshot(snapshot, index);
      const expected=this.verifiedExpected(snapshot,verified,b,index);
      if (verified.bodyHash !== hash(expected) || verified.page.body !== expected ||
          verified.page.properties !== snapshot.page.properties)
        fail('PERSISTENCE', 'Durable verified receipt changed');
      const page = pageData(input.page, b.edits[index].pageId);
      if (page.body !== verified.page.body || page.properties !== verified.page.properties ||
          page.revision !== verified.page.revision)
        fail('RECONCILE_CHANGED', 'An already verified page changed; keep the barrier');
      recovery.next++;
      return { status:'HALTED', checked:index, next:b.next, protectedContent:'UNCHANGED' };
    }
    if (input.ticket !== b.receipt.ticket)
      fail('RECONCILE_IDENTITY', 'Keep the original immutable write ticket');
    if (b.next > 0 && (!this.recovery || this.recovery.batchId !== batchId ||
        this.recovery.planHash !== planHash || this.recovery.expectedError !== expectedError ||
        this.recovery.next !== b.next))
      fail('RECONCILE_ORDER', 'Freshly verify every completed page before resuming');
    const snapshot = read(b.receipt.snapshot);
    if (b.receipt.snapshot !== this.snapshot(batchId, b.next) ||
        hash(JSON.stringify(snapshot)) !== b.receipt.snapshotHash || snapshot.ticket !== input.ticket)
      fail('PERSISTENCE', 'Immutable pending snapshot changed');
    checkSnapshot(snapshot, b.next);
    const page = pageData(input.page, b.edits[b.next].pageId);
    if (page.body !== snapshot.page.body || page.properties !== snapshot.page.properties ||
        page.revision !== snapshot.page.revision)
      fail('RECONCILE_CHANGED', 'Pending page must still equal its full preimage; no write is acknowledged');
    // Persist an audit trail. Do not advance next, invent a verified receipt,
    // remove snapshots, change edits, or accept an already-written output.
    b.reconciliations = [...(b.reconciliations ?? []), {
      kind:'UNWRITTEN_DC_AUTHORIZATION', at:new Date().toISOString(), index:b.next,
      ticket:input.ticket, planHash, error:expectedError, priorVerified:b.next,
      bodyHash:hash(page.body), propertiesHash:hash(page.properties),
      mode:this.recovery?.mode ?? 'FRESH_PRIOR_PAGES', verifiedReceipts:this.recovery?.receipts ?? []
    }];
    b.status = 'PREPARED';
    this.recovery = null;
    this.save();
    return { status:'PREPARED', next:b.next, ticket:input.ticket, protectedContent:'UNCHANGED' };
  }
  handle(input) {
    const { action, batchId } = input;
    if (action === 'status') return clone(this.state);
    if (action === 'close') return { status: 'CLOSED' };
    if (typeof batchId !== 'string' || !/^[a-zA-Z0-9_.:-]{1,150}$/.test(batchId) ||
        ['__proto__', 'prototype', 'constructor'].includes(batchId)) fail('BATCH_ID', 'Stable safe observation ID required');
    if (action === 'begin' || action === 'validate_batch') {
      const edits = input.edits;
      if (!Array.isArray(edits) || !edits.length || edits.length > 30) fail('EDIT_SCHEMA', '1–30 page edits required');
      const seen = new Set();
      for (const edit of edits) {
        validateScope(edit, this.policy);
        const page = id(edit.pageId);
        if (seen.has(page)) fail('DUPLICATE_PAGE', 'One minimal literal per page per batch');
        seen.add(page);
      }
      const key = hash(JSON.stringify(edits));
      const previous = Object.hasOwn(this.state.batches,batchId) ? this.state.batches[batchId] : null;
      if (previous) {
        if (previous.planHash !== key) fail('BATCH_REUSE', 'Observation ID cannot bind different edits');
        if (previous.status === 'READY') previous.edits.slice(previous.next).forEach(stableNewText);
        return { status: previous.status, next: previous.next, reused: true };
      }
      edits.forEach(stableNewText);
      if (this.state.active) fail('UNRESOLVED_BATCH', 'Previous batch must verify or be reconciled before any new batch');
      if(action==='validate_batch')return {status:'READY',next:0};
      this.state.batches[batchId] = { planHash: key, edits: clone(edits), next: 0, status: 'READY', receipt: null };
      this.state.active = batchId; this.save(); return { status: 'READY', next: 0 };
    }
    if(action==='preflight') {
      validateScope(input.edit,this.policy);stableNewText(input.edit);
      const p=plan(pageData(input.page,input.edit.pageId),input.edit);
      return {status:'SAFE_LITERAL',expectedHash:hash(p.expected),beforeHash:p.beforeHash,afterHash:p.afterHash};
    }
    if (action === 'reconcile_autolink') return this.reconcileAutolink(input);
    if (action === 'reconcile_escaped_arrows') return this.reconcileRendering(input, 'arrows');
    if (['reconcile_verified','reconcile_receipts','reconcile_unwritten'].includes(action)) return this.reconcile(input);
    const b = this.current(batchId), edit = b.edits[b.next];
    if (action === 'halt') {
      b.status = 'HALTED'; b.error = String(input.reason ?? 'Connector or runner failed').slice(0,1000);
      this.save(); return { status: 'HALTED', next: b.next };
    }
    if (action === 'prepare') {
      if (b.status !== 'READY') fail('PAGE_BARRIER', 'Prior page has not verified');
      validateScope(edit, this.policy);
      stableNewText(edit);
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
      validateScope(edit, this.policy);
      stableNewText(edit);
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
