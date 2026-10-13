'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const { Broker, hash } = require('./broker.cjs');
const A='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', B='bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
const heading='## CURRENT CENTRAL OBSERVATION · test';
const oldStr='central_status=WAIT', newStr='central_status=VERIFIED';
const history='immutable history 🧾 '.repeat(250);
function body(status=oldStr) { return heading+'\n'+status+'\n## HISTORICAL WORKER PROOF\n'+history+'\n```json\n{"QUEUE_EVENT_V1":true,"event_id":"real","session_id":"worker","lease_state":"RELEASED"}\n```\n<page url="https://notion.so/cccccccccccccccccccccccccccccccc">Child</page>\n'; }
function page(pageId=A, content=body(), props={title:'Test'}, revision='2026-10-12T00:00:00Z') {
  return { structuredContent:{ id:pageId, text:'Fetch at dynamic time\n<page>\n<properties>\n'+JSON.stringify(props)+'\n</properties>\n<content>\n'+content+'\n</content>\n</page>', page_last_edited_at:revision } };
}
function edit(pageId=A, changes={}) { return {pageId,heading,oldStr,newStr,...changes}; }
function fixture(t, policy={}) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'notion-guard-'));
  const p={ productionEnabled:true, productionPages:[A,B],canaryPages:[],headingPrefixes:['## CURRENT CENTRAL OBSERVATION'], ...policy };
  let broker = new Broker(root,p);
  t.after(()=>{ if (fs.existsSync(path.join(root,'broker.lock'))) broker.close(); fs.rmSync(root,{recursive:true,force:true}); });
  return {root,p,get b(){return broker;},restart(){broker.close();broker=new Broker(root,p);return broker;}};
}
function begin(b, edits=[edit()]) { return b.handle({action:'begin',batchId:'observation',edits}); }
function prepare(b, p=page()) { return b.handle({action:'prepare',batchId:'observation',page:p}); }
function authorize(b, receipt, p=page()) { return b.handle({action:'authorize',batchId:'observation',ticket:receipt.ticket,page:p}); }
function verify(b, receipt, p=page(A,body(newStr))) { return b.handle({action:'verify',batchId:'observation',ticket:receipt.ticket,page:p}); }
function rejects(code, fn) { assert.throws(fn, error=>error.code===code); }

test('two real-shaped pages preserve all 5000+ characters, proof and child refs',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);
  const first=prepare(b);assert.equal(fs.existsSync(first.snapshot),true);
  assert.equal(JSON.parse(fs.readFileSync(first.snapshot)).page.body,body());
  assert.deepEqual(authorize(b,first).request.content_updates,[{old_str:oldStr,new_str:newStr,replace_all_matches:false}]);
  assert.equal(verify(b,first).status,'READY');
  const second=prepare(b,page(B));authorize(b,second,page(B));
  assert.equal(verify(b,second,page(B,body(newStr))).status,'COMPLETED');
  assert.equal(b.handle({action:'status'}).active,null);
  assert.equal(begin(b,[edit(),edit(B)]).status,'COMPLETED');
});
test('4000-character destructive selection rejects before preparing any page',t=>{
  const {b}=fixture(t); rejects('RANGE_LIMIT',()=>begin(b,[edit(),edit(B,{oldStr:'x'.repeat(4000)})]));
  assert.equal(b.handle({action:'status'}).active,null);
});
for (const [name,change,code] of [
  ['multiline range',{oldStr:oldStr+'\n## HISTORICAL WORKER PROOF'},'RANGE_LIMIT'],
  ['whole replace parameter',{command:'replace_content'},'EDIT_SCHEMA'],
  ['worker section',{heading:'## HISTORICAL WORKER PROOF'},'OWNERSHIP'],
  ['native child deletion',{oldStr:'<page url="x">Child</page>'},'PROTECTED_SYNTAX'],
  ['forged worker proof',{newStr:'QUEUE_EVENT_V1 RELEASE'},'PROTECTED_SYNTAX'],
  ['new heading injection',{newStr:'## replacement'},'PROTECTED_SYNTAX']
]) test(name,t=>{const {b}=fixture(t);rejects(code,()=>begin(b,[edit(A,change)]));});
test('duplicate literal in immutable history is rejected',t=>{
  const {b}=fixture(t);begin(b);rejects('AMBIGUOUS_LITERAL',()=>prepare(b,page(A,body()+oldStr)));
});
test('literal exclusively outside central section is rejected',t=>{
  const {b}=fixture(t);begin(b,[edit(A,{oldStr:'immutable history',newStr:'changed'})]);
  rejects('AMBIGUOUS_LITERAL',()=>prepare(b));
  rejects('OUTSIDE_SECTION',()=>prepare(b,page(A,heading+'\nuntouched\n## HISTORY\nimmutable history')));
});
test('duplicate central heading is rejected',t=>{
  const {b}=fixture(t);begin(b);rejects('AMBIGUOUS_SECTION',()=>prepare(b,page(A,body()+'\n'+heading)));
});
test('no guessed end boundary at page end',t=>{
  const {b}=fixture(t);begin(b);rejects('UNBOUNDED_SECTION',()=>prepare(b,page(A,heading+'\n'+oldStr)));
});
test('central-looking heading in code or synced block is not an editable section',t=>{
  const {b}=fixture(t);begin(b);
  for (const content of ['```\n'+body()+'\n```','<synced_block url="x">\n'+body()+'\n</synced_block>'])
    rejects('AMBIGUOUS_SECTION',()=>prepare(b,page(A,content)));
});
test('worker event moved into central section is protected',t=>{
  const {b}=fixture(t);begin(b);rejects('PROTECTED_SECTION',()=>prepare(b,page(A,body().replace(oldStr,oldStr+'\nevent_id=worker'))));
});
test('new receipt/properties/revision between reads blocks the write',t=>{
  const {b}=fixture(t);begin(b);const r=prepare(b);
  for(const p of [page(A,body()+'new own RELEASE'),page(A,body(),{title:'Test',session_id:'new'}),page(A,body(),{title:'Test'},'2026-10-12T00:01:00Z')])
    rejects('CONCURRENT_CHANGE',()=>authorize(b,r,p));
  rejects('PAGE_BARRIER',()=>prepare(b,page(B)));
});
test('tail deletion after connector success halts before page two and next batch',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);const r=prepare(b);authorize(b,r);
  rejects('READBACK_MISMATCH',()=>verify(b,r,page(A,body(newStr).replace(history,''))));
  rejects('HALTED',()=>prepare(b,page(B)));
  rejects('UNRESOLVED_BATCH',()=>b.handle({action:'begin',batchId:'later',edits:[edit(B)]}));
  assert.equal(fs.existsSync(r.snapshot+'.actual.json'),true);
});
test('remote normalization is not silently accepted',t=>{
  const {b}=fixture(t);begin(b);const r=prepare(b);authorize(b,r);
  rejects('READBACK_MISMATCH',()=>verify(b,r,page(A,body(newStr).replace('Child','CHILD'))));
});
test('snapshot corruption stops authorization',t=>{
  const {b}=fixture(t);begin(b);const r=prepare(b);const p=JSON.parse(fs.readFileSync(r.snapshot));p.page.body+='tampered';fs.writeFileSync(r.snapshot,JSON.stringify(p));
  rejects('PERSISTENCE',()=>authorize(b,r));
});
test('lost ACK/process restart never authorizes duplicate write or next page',t=>{
  const f=fixture(t);begin(f.b,[edit(),edit(B)]);const r=prepare(f.b);authorize(f.b,r);f.restart();
  rejects('PAGE_BARRIER',()=>authorize(f.b,r));rejects('PAGE_BARRIER',()=>prepare(f.b,page(B)));
  assert.equal(verify(f.b,r).status,'READY');
});
test('unrestored production pages remain blocked',t=>{
  const {b}=fixture(t,{productionEnabled:false});rejects('PRODUCTION_HOLD',()=>begin(b));
});
test('known private canary does not unlock production',t=>{
  const {b}=fixture(t,{productionEnabled:false,canaryPages:[B]});begin(b,[edit(B)]);assert.equal(b.handle({action:'status'}).batches.observation.status,'READY');
});
test('incomplete/unknown read and wrong page are rejected',t=>{
  const {b}=fixture(t);begin(b);
  const p=page();p.structuredContent.truncated=true;rejects('INCOMPLETE_READ',()=>prepare(b,p));
  rejects('INCOMPLETE_READ',()=>prepare(b,page(A,body()+'<unknown url="x"/>')));
  rejects('WRONG_PAGE',()=>prepare(b,page(B)));
});
test('same observation cannot carry changed edit and duplicate pages are rejected',t=>{
  const {b}=fixture(t);rejects('DUPLICATE_PAGE',()=>begin(b,[edit(),edit()]));begin(b);
  rejects('BATCH_REUSE',()=>begin(b,[edit(A,{newStr:'central_status=OTHER'})]));
});
test('another process cannot plan around the global barrier',t=>{
  const f=fixture(t);rejects('LOCKED',()=>new Broker(f.root,f.p));
});
test('snapshot persistence failure yields no write authorization',t=>{
  const f=fixture(t);begin(f.b);const original=fs.renameSync;
  fs.renameSync=()=>{throw new Error('injected disk failure');};
  try {assert.throws(()=>prepare(f.b),/disk failure/);}finally{fs.renameSync=original;}
  assert.equal(f.b.handle({action:'status'}).batches.observation.status,'READY');
});

const createRunner=vm.runInNewContext(fs.readFileSync(path.join(__dirname,'runner.js'),'utf8'),{setTimeout:(fn)=>setTimeout(fn,0)});
function runnerFixture(t, fault) {
  const f=fixture(t);let writes=0,fetches=0,closed=false,retained=null;const pages=new Map([[A,page()],[B,page(B)]]);
  const tools={
    async mcp__codex_apps__remote_desktop_commander_start_process(){return {content:[{type:'text',text:'Process started with PID 123\nNPG/1 {"ready":true}'}]};},
    async mcp__codex_apps__remote_desktop_commander_interact_with_process({input,wait_for_prompt}){
      const r=JSON.parse(input);let value;
      if(fault==='prompt-regex' && r.action==='authorize' && wait_for_prompt!==false)
        return {isError:true,content:[{type:'text',text:'Error interacting with process: Invalid regular expression: /^'+input.slice(0,500)}]};
      try{value={requestId:r.requestId,ok:true,result:f.b.handle(r)};}catch(e){value={requestId:r.requestId,ok:false,code:e.code,error:e.message};}
      if(r.action==='close')closed=true;
      if(fault==='cursor' && r.action==='prepare') {retained=value;return {content:[{type:'text',text:'Input sent; read output'}]};}
      return {content:[{type:'text',text:'NPG/1 '+JSON.stringify(value)}]};
    },
    async mcp__codex_apps__remote_desktop_commander_read_process_output({offset}){
      return {content:[{type:'text',text:offset<0?'  NPG/1 '+JSON.stringify(retained):'No new output'}]};
    },
    async mcp__codex_apps__notion_fetch({id}){
      fetches++;const value=structuredClone(pages.get(id));
      if(fault==='readback' && writes===1)value.structuredContent.text=value.structuredContent.text.replace(history,'');
      return value;
    },
    async mcp__codex_apps__notion_notion_update_page(request){
      writes++;const value=pages.get(request.page_id);const e=request.content_updates[0];value.structuredContent.text=value.structuredContent.text.replace(e.old_str,e.new_str);
      if(fault==='lost-ack')throw new Error('connection lost after commit');
      if(fault==='async')return {content:[{type:'text',text:'Async'}],structuredContent:{async_task:{task_id:'task_1'}}};
      if(fault==='content-only')return {content:[{type:'text',text:JSON.stringify({page_id:request.page_id})}]};
      return {content:[{type:'text',text:'Success'}],structuredContent:{page_id:request.page_id}};
    },
    async mcp__codex_apps__notion_notion_get_async_task(){return {content:[{type:'text',text:'Failed'}],structuredContent:{status:'failed'}};}
  };
  return {runner:createRunner(tools,{deviceId:'known',command:'known'}),newRunner:()=>createRunner(tools,{deviceId:'known',command:'known'}),get writes(){return writes;},get fetches(){return fetches;},get closed(){return closed;},pages,f,tools};
}
test('real orchestration uses broker requests, fresh pre/post reads and sequential pages',async t=>{
  const f=runnerFixture(t);const r=await f.runner.run('observation',[edit(),edit(B)]);
  assert.equal(r.status,'COMPLETED');assert.equal(f.writes,2);assert.equal(f.fetches,6);assert.equal(f.closed,true);
});
for(const fault of ['readback','lost-ack','async'])test('connector '+fault+' prevents second Notion mutation',async t=>{
  const f=runnerFixture(t,fault);await assert.rejects(f.runner.run('observation',[edit(),edit(B)]));
  assert.equal(f.writes,1);assert.equal(f.closed,true);assert.equal(f.pages.get(B).structuredContent.text.includes(oldStr),true);
  assert.equal(f.f.b.handle({action:'status'}).batches.observation.status,'HALTED');
});
test('completed observation replays no mutation even for prefix expansion',async t=>{
  const f=runnerFixture(t);const edits=[edit(A,{newStr:oldStr+' then verified'})];await f.runner.run('observation',edits);
  assert.equal((await f.newRunner().run('observation',edits)).alreadyConsumed,true);assert.equal(f.writes,1);
});
test('DC consumed cursor: retained response by exact ID avoids resend and permits verification',async t=>{
  const f=runnerFixture(t,'cursor');assert.equal((await f.runner.run('observation',[edit()])).status,'COMPLETED');
  assert.equal(f.writes,1);assert.equal(f.fetches,3);assert.equal(f.closed,true);
});
test('native Notion content-only JSON ACK is recognized and then fully read back',async t=>{
  const f=runnerFixture(t,'content-only');assert.equal((await f.runner.run('observation',[edit(),edit(B)])).status,'COMPLETED');
  assert.equal(f.writes,2);assert.equal(f.fetches,6);
});

test('large JSON with regex metacharacters never enters DC prompt matching',async t=>{
  const f=runnerFixture(t,'prompt-regex');
  f.pages.set(A,page(A,body()+('[{\\ ⭐ 한국어 '.repeat(150000))));
  assert.equal((await f.runner.run('observation',[edit()])).status,'COMPLETED');
  assert.equal(f.writes,1);
});

function haltAuthorization(b, receipt) {
  const error='REMOTE_ERROR: Error interacting with process: Invalid regular expression: /^\\{"action":"authorize","batchId":"observation","ticket":'+JSON.stringify(receipt.ticket);
  b.handle({action:'halt',batchId:'observation',reason:error});
  return error;
}
function repairInput(b, action, extra={}) {
  const state=b.handle({action:'status'}).batches.observation;
  return {action,batchId:'observation',planHash:state.planHash,expectedError:state.error,...extra};
}
test('explicit recovery keeps the same canonical batch, ticket and page index',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);
  const first=prepare(b);authorize(b,first);verify(b,first);
  const pending=prepare(b,page(B));haltAuthorization(b,pending);
  rejects('RECONCILE_ORDER',()=>b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)})));
  b.handle(repairInput(b,'reconcile_verified',{index:0,page:page(A,body(newStr))}));
  const result=b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)}));
  assert.equal(result.status,'PREPARED');assert.equal(result.next,1);
  assert.equal(b.state.batches.observation.receipt.ticket,pending.ticket);
  assert.equal(b.state.batches.observation.reconciliations[0].priorVerified,1);
  authorize(b,pending,page(B));assert.equal(verify(b,pending,page(B,body(newStr))).status,'COMPLETED');
});
test('completed receipt/protected properties drift blocks recovery without clearing HALTED',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);const a=prepare(b);authorize(b,a);verify(b,a);
  const pending=prepare(b,page(B));haltAuthorization(b,pending);
  for(const p of [page(A,body(newStr)+'new own RELEASE'),page(A,body(newStr),{title:'changed'})])
    rejects('RECONCILE_CHANGED',()=>b.handle(repairInput(b,'reconcile_verified',{index:0,page:p})));
  assert.equal(b.state.batches.observation.status,'HALTED');
});
test('changed current preimage, revision or applied output never reauthorizes',t=>{
  const {b}=fixture(t);begin(b);const pending=prepare(b);haltAuthorization(b,pending);
  for(const p of [page(A,body()+'new own RELEASE'),page(A,body(),{title:'changed'}),page(A,body(),{title:'Test'},'later'),page(A,body(newStr))])
    rejects('RECONCILE_CHANGED',()=>b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:p})));
  assert.equal(b.state.batches.observation.status,'HALTED');assert.equal(b.state.batches.observation.next,0);
});
test('other failures, changed plan/error/ticket and tampered snapshots remain blocked',t=>{
  const {b}=fixture(t);begin(b);const pending=prepare(b);haltAuthorization(b,pending);
  const good=repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page()});
  for(const changes of [{planHash:'changed'},{expectedError:'changed'},{ticket:'changed'}])
    rejects('RECONCILE_IDENTITY',()=>b.handle({...good,...changes}));
  const snapshot=JSON.parse(fs.readFileSync(pending.snapshot));snapshot.page.body+='changed';fs.writeFileSync(pending.snapshot,JSON.stringify(snapshot));
  rejects('PERSISTENCE',()=>b.handle(good));
  b.state.batches.observation.error='READBACK_MISMATCH';
  rejects('RECONCILE_SCOPE',()=>b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page()})));
  assert.equal(b.state.batches.observation.status,'HALTED');
});
test('recovery after restart must fresh-check every earlier verified page again',t=>{
  const f=fixture(t);begin(f.b,[edit(),edit(B)]);const a=prepare(f.b);authorize(f.b,a);verify(f.b,a);
  const pending=prepare(f.b,page(B));haltAuthorization(f.b,pending);
  f.b.handle(repairInput(f.b,'reconcile_verified',{index:0,page:page(A,body(newStr))}));f.restart();
  rejects('RECONCILE_ORDER',()=>f.b.handle(repairInput(f.b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)})));
});
test('runner explicitly resumes same halted authorization with one remaining write',async t=>{
  const f=runnerFixture(t);begin(f.f.b,[edit(),edit(B)]);
  const a=prepare(f.f.b);authorize(f.f.b,a);verify(f.f.b,a);f.pages.set(A,page(A,body(newStr)));
  const pending=prepare(f.f.b,page(B));haltAuthorization(f.f.b,pending);
  const planHash=f.f.b.state.batches.observation.planHash;
  const result=await f.runner.resume('observation',planHash);
  assert.equal(result.status,'COMPLETED');assert.equal(f.writes,1);assert.equal(f.closed,true);
});

test('resume preserves later worker records on a completed page without rewriting it',async t=>{
  const f=runnerFixture(t);begin(f.f.b,[edit(),edit(B)]);
  const a=prepare(f.f.b);authorize(f.f.b,a);verify(f.f.b,a);
  const evolved=page(A,body(newStr)+'new genuine worker RELEASE v32',{title:'Test',session_id:'new-worker'},'2026-10-13T12:18:30Z');
  f.pages.set(A,evolved);
  const pending=prepare(f.f.b,page(B));haltAuthorization(f.f.b,pending);
  const result=await f.runner.resume('observation',f.f.b.state.batches.observation.planHash);
  assert.equal(result.status,'COMPLETED');assert.equal(f.writes,1);
  assert.deepEqual(f.pages.get(A),evolved);
  assert.equal(f.f.b.state.batches.observation.reconciliations[0].mode,'DURABLE_VERIFIED_RECEIPTS');
});
test('receipt-only recovery checks immutable evidence without issuing any old-page write',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);
  const a=prepare(b);authorize(b,a);verify(b,a);
  const pending=prepare(b,page(B));haltAuthorization(b,pending);
  const checked=b.handle(repairInput(b,'reconcile_receipts'));
  assert.equal(checked.status,'HALTED');assert.equal(checked.priorVerified,1);
  assert.equal(b.state.batches.observation.next,1);
  const ready=b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)}));
  assert.equal(ready.status,'PREPARED');assert.equal(ready.next,1);
});
test('a damaged durable verified receipt cannot unlock the current pending page',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);
  const a=prepare(b);authorize(b,a);verify(b,a);
  const pending=prepare(b,page(B));haltAuthorization(b,pending);
  const v=JSON.parse(fs.readFileSync(a.snapshot+'.verified.json'));v.page.body=v.page.body.replace(history,'');
  fs.writeFileSync(a.snapshot+'.verified.json',JSON.stringify(v));
  rejects('PERSISTENCE',()=>b.handle(repairInput(b,'reconcile_receipts')));
  assert.equal(b.state.batches.observation.status,'HALTED');
  rejects('RECONCILE_ORDER',()=>b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)})));
});
test('fresh pending-page drift after receipt checks blocks without acknowledging a write',t=>{
  const {b}=fixture(t);begin(b,[edit(),edit(B)]);
  const a=prepare(b);authorize(b,a);verify(b,a);
  const pending=prepare(b,page(B));haltAuthorization(b,pending);
  b.handle(repairInput(b,'reconcile_receipts'));
  rejects('RECONCILE_CHANGED',()=>b.handle(repairInput(b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B,body(),{title:'new own record'})})));
  assert.equal(b.state.batches.observation.status,'HALTED');assert.equal(b.state.batches.observation.next,1);
});
