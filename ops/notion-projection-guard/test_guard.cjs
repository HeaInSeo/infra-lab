'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const { Broker, hash, pageData, plan, atomic } = require('./broker.cjs');
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

const w02Sections = [
  {pageId:'3f586e6dd1d181ebb43dd7de62aa2ca4',heading:'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3'},
  {pageId:'3f586e6dd1d181cd9f5bdcdab6290de7',heading:'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED'}
];
const ownPreflight = '## OPERATOR PREFLIGHT NOTE (no CLAIM, no lease)\nworker_note=DESIGN_GAP; original evidence preserved\n';
function w02Body(section, status=oldStr) { return ownPreflight+section.heading+'\n'+status+'\n## 착수·범위·증거\n'+body('historical_status=KEPT'); }
function w02Edit(section, changes={}) { return {...section,oldStr,newStr,...changes}; }
for (const section of w02Sections) {
  test('exact W02 central section preserves earlier preflight, later proof and all properties: '+section.pageId,t=>{
    const {b}=fixture(t,{centralSections:w02Sections});
    const props={title:'W02', '세션 ID':'genuine-worker', '세션 단계':'미시작', '이벤트 ID':'original'};
    const input=page(section.pageId,w02Body(section),props);
    begin(b,[w02Edit(section)]);const r=prepare(b,input);
    const request=authorize(b,r,input).request;
    assert.deepEqual(request,{page_id:section.pageId,command:'update_content',allow_async:false,allow_deleting_content:false,
      content_updates:[{old_str:oldStr,new_str:newStr,replace_all_matches:false}]});
    assert.equal(verify(b,r,page(section.pageId,w02Body(section,newStr),props)).protectedContent,'UNCHANGED');
    assert.equal(begin(b,[w02Edit(section)]).status,'COMPLETED');
  });
  for (const change of [
    {heading:section.heading+' extra'}, {heading:w02Sections.find(s=>s!==section).heading},
    {heading:'## CURRENT'}, {heading}, {heading:'## OPERATOR PREFLIGHT NOTE (no CLAIM, no lease)'},
    {pageId:A}, {pageId:'cccccccccccccccccccccccccccccccc'}
  ]) test('W02 permission does not expand to '+JSON.stringify(change),t=>{
    const {b}=fixture(t,{centralSections:w02Sections,productionPages:[A,B,...w02Sections.map(s=>s.pageId)]});
    rejects('OWNERSHIP',()=>begin(b,[w02Edit(section,change)]));assert.equal(b.state.active,null);
  });
  test('W02 central scope requires opt-in and an enabled production policy: '+section.pageId,t=>{
    const f=fixture(t);rejects('OWNERSHIP',()=>begin(f.b,[w02Edit(section)]));
    f.p.centralSections=w02Sections;f.p.productionEnabled=false;
    rejects('PRODUCTION_HOLD',()=>begin(f.b,[w02Edit(section)]));assert.equal(f.b.state.active,null);
  });
  test('W02 worker edit and property injection are rejected: '+section.pageId,t=>{
    const {b}=fixture(t,{centralSections:w02Sections});
    rejects('EDIT_SCHEMA',()=>begin(b,[w02Edit(section,{properties:{'세션 ID':'forged'}})]));
    begin(b,[w02Edit(section,{oldStr:'worker_note=DESIGN_GAP',newStr:'worker_note=READY'})]);
    rejects('OUTSIDE_SECTION',()=>prepare(b,page(section.pageId,w02Body(section))));
  });
  for (const marker of ['event_id=worker','```json\n{"QUEUE_EVENT_V1":true}\n```','### own evidence','<page url="https://notion.so/'+A+'">Child</page>'])
    test('W02 central section containing protected structure rejects: '+section.pageId+' '+marker,t=>{
      const {b}=fixture(t,{centralSections:w02Sections});begin(b,[w02Edit(section)]);
      rejects('PROTECTED_SECTION',()=>prepare(b,page(section.pageId,w02Body(section).replace(oldStr,oldStr+'\n'+marker))));
    });
  test('W02 scope is rechecked after policy revocation at prepare and authorize: '+section.pageId,t=>{
    const f=fixture(t,{centralSections:w02Sections});begin(f.b,[w02Edit(section)]);
    f.p.centralSections=[];f.restart();
    rejects('OWNERSHIP',()=>prepare(f.b,page(section.pageId,w02Body(section))));
    f.p.centralSections=w02Sections;const r=prepare(f.b,page(section.pageId,w02Body(section)));
    f.p.centralSections=[];f.restart();
    rejects('OWNERSHIP',()=>authorize(f.b,r,page(section.pageId,w02Body(section))));
    assert.equal(f.b.state.batches.observation.status,'PREPARED');
    f.p.centralSections=w02Sections;f.p.productionEnabled=false;f.restart();
    rejects('PRODUCTION_HOLD',()=>authorize(f.b,r,page(section.pageId,w02Body(section))));
  });
  test('W02 concurrent preflight, properties or revision stops authorization: '+section.pageId,t=>{
    const {b}=fixture(t,{centralSections:w02Sections});begin(b,[w02Edit(section)]);
    const input=page(section.pageId,w02Body(section));const r=prepare(b,input);
    for (const changed of [page(section.pageId,w02Body(section)+'\nnew worker note'),
      page(section.pageId,w02Body(section),{title:'Test','세션 ID':'new worker'}),
      page(section.pageId,w02Body(section),{title:'Test'},'2026-10-19T00:00:00Z')])
      rejects('CONCURRENT_CHANGE',()=>authorize(b,r,changed));
    assert.equal(b.state.batches.observation.status,'PREPARED');
  });
}
test('W02 prefix/suffix loss or worker property changes halt before the second W02 page',t=>{
  for (const damage of ['prefix','suffix','properties']) {
    const {b}=fixture(t,{centralSections:w02Sections});begin(b,w02Sections.map(s=>w02Edit(s)));
    const s=w02Sections[0],input=page(s.pageId,w02Body(s)),r=prepare(b,input);authorize(b,r,input);
    const content=w02Body(s,newStr);
    const changed=page(s.pageId,damage==='prefix'?content.replace(ownPreflight,''):damage==='suffix'?content.replace(history,''):content,
      damage==='properties'?{title:'Test','세션 ID':'changed'}:{title:'Test'});
    rejects('READBACK_MISMATCH',()=>verify(b,r,changed));
    rejects('HALTED',()=>prepare(b,page(w02Sections[1].pageId,w02Body(w02Sections[1]))));
    rejects('UNRESOLVED_BATCH',()=>b.handle({action:'begin',batchId:'other',edits:[edit()]}));
  }
});
test('W02 runner uses only two exact content requests and preserves both complete pages',async t=>{
  const f=runnerFixture(t,null,{centralSections:w02Sections});
  for(const s of w02Sections) f.pages.set(s.pageId,page(s.pageId,w02Body(s)));
  const result=await f.runner.run('observation',w02Sections.map(s=>w02Edit(s)));
  assert.equal(result.status,'COMPLETED');assert.equal(f.writes,2);assert.equal(f.fetches,6);
  for(const s of w02Sections) assert.equal(pageData(f.pages.get(s.pageId),s.pageId).body,w02Body(s,newStr));
});
for (const sections of [null,{},[w02Sections[0],w02Sections[0]],
  [{...w02Sections[0],pageId:A}],[{...w02Sections[0],heading:'## CURRENT'}],
  [{...w02Sections[0],extra:true}],[{...w02Sections[0],pageId:w02Sections[0].pageId.toUpperCase()}],
  [...w02Sections,w02Sections[0]]]) test('invalid W02 policy rejects before creating state: '+JSON.stringify(sections),t=>{
    const f=fixture(t);const root=path.join(f.root,'invalid');
    rejects('POLICY',()=>new Broker(root,{...f.p,centralSections:sections}));assert.equal(fs.existsSync(root),false);
  });
test('W02 task pages cannot be relabeled as canaries to bypass the production hold',t=>{
  const f=fixture(t);rejects('POLICY',()=>new Broker(path.join(f.root,'invalid'),{...f.p,centralSections:w02Sections,
    productionEnabled:false,canaryPages:[w02Sections[0].pageId]}));
});

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
function runnerFixture(t, fault, policy={}) {
  const f=fixture(t,policy);let writes=0,fetches=0,closed=false,retained=null;const pages=new Map([[A,page()],[B,page(B)]]);
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

function legacyAutolink(b, edits=[edit(A,{newStr:'central_status=package-cli.sh awaiting CI'})], actualBody) {
  // A journal produced before AUTOLINK_RISK existed; upgrade the real persisted
  // schema rather than allowing the new begin() to authorize this legacy text.
  b.state.batches.observation={planHash:hash(JSON.stringify(edits)),edits:structuredClone(edits),
    next:0,status:'READY',receipt:null};b.state.active='observation';b.save();
  // Seed the exact WRITE_PENDING snapshot made by the old binary. The new
  // binary must not prepare or authorize this unsafe text again.
  const preimage=pageData(page(),A),planned=plan(preimage,edits[0]);
  const snapshot={version:1,batchId:'observation',index:0,ticket:'legacy-ticket',edit:edits[0],page:preimage,
    expected:planned.expected,beforeHash:planned.beforeHash,afterHash:planned.afterHash,
    bodyHash:hash(preimage.body),expectedHash:hash(planned.expected),propertiesHash:hash(preimage.properties)};
  const file=b.snapshot('observation',0);atomic(file,snapshot);
  const receipt={ticket:snapshot.ticket,snapshot:file,snapshotHash:hash(JSON.stringify(snapshot))};
  b.state.batches.observation.status='WRITE_PENDING';b.state.batches.observation.receipt=receipt;b.save();
  const rendered=edits[0].newStr.replace('package-cli.sh','[package-cli.sh](http://package-cli.sh)');
  const p=page(A,actualBody??body(rendered),{title:'Test'},'2026-10-12T00:01:00Z');
  rejects('READBACK_MISMATCH',()=>verify(b,receipt,p));
  const batch=b.state.batches.observation;
  const actual=JSON.parse(fs.readFileSync(receipt.snapshot+'.actual.json'));
  return {p,receipt,input:{action:'reconcile_autolink',batchId:'observation',planHash:batch.planHash,
    expectedError:'READBACK_MISMATCH',ticket:receipt.ticket,snapshotHash:batch.receipt.snapshotHash,
    actualHash:actual.actualHash,actualSnapshotHash:hash(JSON.stringify(actual)),page:p}};
}
for (const spelling of ['aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', A.toUpperCase()]) {
  test('unwritten recovery accepts the original admitted UUID spelling: '+spelling,t=>{
    const {b}=fixture(t);begin(b,[edit(spelling)]);
    const pending=prepare(b);haltAuthorization(b,pending);
    b.handle(repairInput(b,'reconcile_receipts'));
    assert.equal(b.handle(repairInput(b,'reconcile_unwritten',{
      ticket:pending.ticket,page:page()})).status,'PREPARED');
    assert.equal(b.state.batches.observation.edits[0].pageId,spelling);
    authorize(b,pending);assert.equal(verify(b,pending).status,'COMPLETED');
  });
  test('autolink recovery accepts the original admitted UUID spelling: '+spelling,t=>{
    const {b}=fixture(t);const legacy=legacyAutolink(b,[edit(spelling,{
      newStr:'central_status=package-cli.sh awaiting CI'})]);
    const result=b.handle(legacy.input);
    assert.equal(result.status,'COMPLETED');assert.equal(result.reconciliation.notionWrites,0);
    assert.equal(b.state.batches.observation.edits[0].pageId,spelling);
  });
}
test('reviewed legacy autolink recovery verifies complete saved and fresh pages without a write',t=>{
  const f=fixture(t);const legacy=legacyAutolink(f.b);f.restart();
  const out=f.b.handle(legacy.input);
  assert.equal(out.status,'COMPLETED');assert.equal(out.reconciliation.notionWrites,0);
  assert.equal(f.b.state.active,null);assert.equal(f.b.state.batches.observation.next,1);
  const verified=JSON.parse(fs.readFileSync(legacy.receipt.snapshot+'.verified.json'));
  assert.equal(verified.page.body,legacy.p.structuredContent.text.split('<content>\n')[1].split('\n</content>')[0]);
  assert.equal(verified.reconciliation.plannedExpectedHash,
    JSON.parse(fs.readFileSync(legacy.receipt.snapshot)).expectedHash);
  rejects('RECONCILE_IDENTITY',()=>f.b.handle(legacy.input));
});
test('legacy autolink recovery rejects fresh body, properties and revision drift while preserving HALTED',t=>{
  const {b}=fixture(t);const legacy=legacyAutolink(b);
  const current=legacy.p.structuredContent.text.split('<content>\n')[1].split('\n</content>')[0];
  for(const p of [page(A,current+'new real RELEASE'),page(A,current,{title:'changed'}),
    page(A,current,{title:'Test'},'2026-10-12T00:02:00Z')])
    rejects('RECONCILE_CHANGED',()=>b.handle({...legacy.input,page:p}));
  assert.equal(b.state.batches.observation.status,'HALTED');assert.equal(b.state.batches.observation.next,0);
  assert.equal(fs.existsSync(legacy.receipt.snapshot+'.verified.json'),false);
});
for(const [name,render] of [
  ['tail loss',s=>s.replace(history,'')],
  ['different link destination',s=>s.replace('http://package-cli.sh','http://example.com')],
  ['extra change inside intended literal',s=>s.replace('awaiting CI','approved')],
  ['worker proof change',s=>s.replace('"session_id":"worker"','"session_id":"forged"')]
])test('reviewed recovery does not admit '+name,t=>{
  const {b}=fixture(t);const intended=body('central_status=[package-cli.sh](http://package-cli.sh) awaiting CI');
  const legacy=legacyAutolink(b,undefined,render(intended));
  rejects('RECONCILE_SCOPE',()=>b.handle(legacy.input));assert.equal(b.state.active,'observation');
});
test('reviewed recovery pins batch plan, ticket, both complete snapshots and original error',t=>{
  const {b}=fixture(t);const legacy=legacyAutolink(b);
  for(const changes of [{planHash:'wrong'},{ticket:'wrong'},{snapshotHash:'wrong'},{expectedError:'other'}])
    rejects('RECONCILE_IDENTITY',()=>b.handle({...legacy.input,...changes}));
  for(const changes of [{actualHash:'wrong'},{actualSnapshotHash:'wrong'}])
    rejects('PERSISTENCE',()=>b.handle({...legacy.input,...changes}));
  const p=JSON.parse(fs.readFileSync(legacy.receipt.snapshot+'.actual.json'));p.at='tampered';
  fs.writeFileSync(legacy.receipt.snapshot+'.actual.json',JSON.stringify(p));
  rejects('PERSISTENCE',()=>b.handle(legacy.input));assert.equal(b.state.batches.observation.status,'HALTED');
});
test('snapshot tampering, production hold and multi-page legacy plans cannot be acknowledged',t=>{
  const f=fixture(t);const legacy=legacyAutolink(f.b);f.p.productionEnabled=false;
  rejects('PRODUCTION_HOLD',()=>f.b.handle(legacy.input));f.p.productionEnabled=true;
  const snap=JSON.parse(fs.readFileSync(legacy.receipt.snapshot));snap.expected+='changed';
  fs.writeFileSync(legacy.receipt.snapshot,JSON.stringify(snap));
  rejects('PERSISTENCE',()=>f.b.handle(legacy.input));
  const other=fixture(t);const multi=legacyAutolink(other.b,
    [edit(A,{newStr:'central_status=package-cli.sh awaiting CI'}),edit(B)]);
  rejects('RECONCILE_IDENTITY',()=>other.b.handle(multi.input));
});
test('hostname-shaped new text rejects before the first fetch and before claiming the durable barrier',async t=>{
  const f=runnerFixture(t);await assert.rejects(f.runner.run('new',[edit(A,{newStr:'tool=package-cli.sh'})]),/AUTOLINK_RISK/);
  assert.equal(f.fetches,0);assert.equal(f.writes,0);assert.equal(f.f.b.state.active,null);
});
test('explicit code or Markdown links are admitted, escaped and unclosed markup is not',t=>{
  for(const s of ['tool=`package-cli.sh`','tool=``package-cli.sh``','tool=[package-cli.sh](http://package-cli.sh)']) {
    const {b}=fixture(t);assert.equal(begin(b,[edit(A,{newStr:s})]).status,'READY');
  }
  for(const s of ['tool=package-cli.sh','tool=`package-cli.sh','tool=\\`package-cli.sh\\`',
    'tool=\\[package-cli.sh](http://package-cli.sh)','tool=https://example.com/file']) {
    const {b}=fixture(t);rejects('AUTOLINK_RISK',()=>begin(b,[edit(A,{newStr:s})]));assert.equal(b.state.active,null);
  }
});
test('runner explicit autolink reconciliation performs one fresh fetch and zero Notion mutations',async t=>{
  const f=runnerFixture(t);const legacy=legacyAutolink(f.f.b);f.pages.set(A,legacy.p);
  const i=legacy.input;
  const result=await f.runner.reconcileAutolink(i.batchId,i.planHash,i.snapshotHash,i.actualHash,i.actualSnapshotHash);
  assert.equal(result.status,'COMPLETED');assert.equal(f.fetches,1);assert.equal(f.writes,0);assert.equal(f.closed,true);
});
test('legacy READY replay cannot bypass autolink preflight or issue a Notion mutation',async t=>{
  const f=runnerFixture(t);const edits=[edit(A,{newStr:'central_status=package-cli.sh awaiting CI'})];
  f.f.b.state.batches.observation={planHash:hash(JSON.stringify(edits)),edits,next:0,status:'READY',receipt:null};
  f.f.b.state.active='observation';f.f.b.save();
  await assert.rejects(f.runner.run('observation',edits),/AUTOLINK_RISK/);
  assert.equal(f.fetches,0);assert.equal(f.writes,0);assert.equal(f.f.b.state.batches.observation.status,'READY');
});
test('direct prepare and legacy PREPARED authorization both reject unsafe text before write authorization',t=>{
  const f=fixture(t);const legacy=legacyAutolink(f.b);const b=f.b.state.batches.observation;
  b.status='READY';b.receipt=null;f.b.save();
  rejects('AUTOLINK_RISK',()=>prepare(f.b));assert.equal(b.status,'READY');
  b.status='PREPARED';b.receipt=legacy.receipt;f.b.save();
  rejects('AUTOLINK_RISK',()=>authorize(f.b,legacy.receipt));assert.equal(b.status,'PREPARED');
});
test('completed legacy autolink batch replays as consumed without rejecting or rewriting it',t=>{
  const {b}=fixture(t);const legacy=legacyAutolink(b);const edits=structuredClone(b.state.batches.observation.edits);
  b.handle(legacy.input);assert.equal(begin(b,edits).status,'COMPLETED');assert.equal(b.state.active,null);
});
test('READY replay validates remaining pages while preserving a previously completed safe page',t=>{
  const {b}=fixture(t);const edits=[edit(),edit(B,{newStr:'central_status=package-cli.sh awaiting CI'})];
  b.state.batches.observation={planHash:hash(JSON.stringify(edits)),edits,next:1,status:'READY',receipt:null};
  b.state.active='observation';b.save();
  rejects('AUTOLINK_RISK',()=>begin(b,edits));assert.equal(b.state.batches.observation.next,1);
});

const arrowIds=[A,B,...['c','d','e','f'].map(c=>c.repeat(32))];
function legacyArrows(b, render=s=>s.replaceAll(' -> ',' -\\> ')) {
  const edits=arrowIds.map((p,i)=>edit(p,{newStr:i===0?'central_status=writer -> review -> landing':newStr}));
  const legacy=legacyAutolink(b,edits,render(body(edits[0].newStr)));
  legacy.input.action='reconcile_escaped_arrows';return {...legacy,edits};
}
test('plain ASCII arrows reject before fetch or durable admission; code and Unicode arrows remain stable',async t=>{
  const f=runnerFixture(t);
  await assert.rejects(f.runner.run('arrows',[edit(A,{newStr:'central_status=writer -> review'})]),/MARKDOWN_ESCAPE_RISK/);
  assert.equal(f.fetches,0);assert.equal(f.writes,0);assert.equal(f.f.b.state.active,null);
  for(const s of ['central_status=writer → review','central_status=`writer -> review`']) {
    const {b}=fixture(t);assert.equal(begin(b,[edit(A,{newStr:s})]).status,'READY');
  }
});
test('explicit arrow recovery verifies page zero, then the original plan writes only the remaining five pages',async t=>{
  const f=runnerFixture(t,undefined,{productionPages:arrowIds});const legacy=legacyArrows(f.f.b);
  f.pages.set(A,legacy.p);for(const p of arrowIds.slice(1))f.pages.set(p,page(p));
  const recovered=await f.runner.broker(legacy.input);
  assert.equal(recovered.status,'READY');assert.equal(recovered.next,1);
  assert.equal(recovered.reconciliation.notionWrites,0);assert.equal(f.writes,0);
  assert.equal((await f.runner.run('observation',legacy.edits)).status,'COMPLETED');
  assert.equal(f.writes,5);assert.deepEqual(f.pages.get(A),legacy.p);
});
for(const [name,render] of [
  ['only one arrow escaped',s=>s.replace(' -> ',' -\\> ')],
  ['protected tail lost',s=>s.replaceAll(' -> ',' -\\> ').replace(history,'')],
  ['literal wording changed',s=>s.replaceAll(' -> ',' -\\> ').replace('review','approved')]
])test('arrow recovery rejects '+name,t=>{
  const {b}=fixture(t,{productionPages:arrowIds});const legacy=legacyArrows(b,render);
  rejects('RECONCILE_SCOPE',()=>b.handle(legacy.input));assert.equal(b.state.batches.observation.status,'HALTED');
});
test('arrow recovery pins fresh revision, policy, original evidence and six-page scope',t=>{
  const {b,p}=fixture(t,{productionPages:arrowIds});const legacy=legacyArrows(b);
  const text=legacy.p.structuredContent.text.split('<content>\n')[1].split('\n</content>')[0];
  rejects('RECONCILE_CHANGED',()=>b.handle({...legacy.input,page:page(A,text,{title:'Test'},'later')}));
  rejects('PERSISTENCE',()=>b.handle({...legacy.input,actualSnapshotHash:'wrong'}));
  p.productionPages=[A];rejects('PRODUCTION_HOLD',()=>b.handle(legacy.input));p.productionPages=arrowIds;
  const edits=b.state.batches.observation.edits;edits.pop();
  b.state.batches.observation.planHash=hash(JSON.stringify(edits));
  rejects('RECONCILE_IDENTITY',()=>b.handle({...legacy.input,planHash:b.state.batches.observation.planHash}));
});
test('a later unwritten transport recovery rechecks the audited arrow receipt without rewriting its page',t=>{
  const f=fixture(t,{productionPages:arrowIds});const legacy=legacyArrows(f.b);f.b.handle(legacy.input);
  const pending=prepare(f.b,page(B));haltAuthorization(f.b,pending);f.restart();
  assert.equal(f.b.handle(repairInput(f.b,'reconcile_receipts')).priorVerified,1);
  assert.equal(f.b.handle(repairInput(f.b,'reconcile_unwritten',{ticket:pending.ticket,page:page(B)})).status,'PREPARED');
  const verified=JSON.parse(fs.readFileSync(legacy.receipt.snapshot+'.verified.json'));
  verified.reconciliation.actualHash='tampered';fs.writeFileSync(legacy.receipt.snapshot+'.verified.json',JSON.stringify(verified));
  haltAuthorization(f.b,pending);
  rejects('PERSISTENCE',()=>f.b.handle(repairInput(f.b,'reconcile_receipts')));
});
test('resume identity rejection preserves an already reconciled READY batch for ordinary run',async t=>{
  const f=runnerFixture(t,undefined,{productionPages:arrowIds});const legacy=legacyArrows(f.f.b);
  f.pages.set(A,legacy.p);for(const p of arrowIds.slice(1))f.pages.set(p,page(p));
  f.f.b.handle(legacy.input);
  await assert.rejects(f.runner.resume('observation',legacy.input.planHash),/RECONCILE_IDENTITY/);
  assert.equal(f.f.b.state.batches.observation.status,'READY');
  assert.equal(f.f.b.state.batches.observation.next,1);assert.equal(f.writes,0);
  assert.equal((await f.newRunner().run('observation',legacy.edits)).status,'COMPLETED');
  assert.equal(f.writes,5);assert.deepEqual(f.pages.get(A),legacy.p);
});
test('inline code masks literal backslashes and shorter backtick runs before rendering preflight',t=>{
  for(const s of ['path=`C:\\tmp\\package-cli.sh -> next`',
    'path=`C:\\tmp\\` then → next', 'path=``C:\\tmp `inner` -> next``']) {
    const {b}=fixture(t);assert.equal(begin(b,[edit(A,{newStr:s})]).status,'READY');
  }
  const {b}=fixture(t);
  rejects('MARKDOWN_ESCAPE_RISK',()=>begin(b,[edit(A,{newStr:'path=\\`C:\\tmp -> next\\`'})]));
});
test('run identity rejection also preserves a reconciled READY plan for the correct caller',async t=>{
  const f=runnerFixture(t,undefined,{productionPages:arrowIds});const legacy=legacyArrows(f.f.b);
  f.pages.set(A,legacy.p);for(const p of arrowIds.slice(1))f.pages.set(p,page(p));
  f.f.b.handle(legacy.input);
  const wrong=structuredClone(legacy.edits);wrong[1].newStr='central_status=DIFFERENT';
  await assert.rejects(f.runner.run('observation',wrong),/BATCH_REUSE/);
  assert.equal(f.f.b.state.batches.observation.status,'READY');assert.equal(f.writes,0);
  assert.equal((await f.newRunner().run('observation',legacy.edits)).status,'COMPLETED');assert.equal(f.writes,5);
});

require('./router_stale_prewrite_cases.cjs');
require('./transport_preprepare_cases.cjs');
require('./complete_projection_cases.cjs');

