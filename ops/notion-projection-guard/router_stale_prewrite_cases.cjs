'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),vm=require('node:vm');
const guard=require('./broker.cjs');
const {Broker,hash,pageData,atomic}=guard;
const BATCH='QSYNC-20261019T082053Z-SORI42-RELEASE-ADMISSION';
const ERROR='AMBIGUOUS_LITERAL: Old literal must occur exactly once in the entire body';
const IDS=['3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7',
  '3f586e6dd1d181ddb9f3e7217f64bfcd','3b286e6dd1d181f2a75cfbd3885071c4','3bf86e6dd1d18110938bc358e17c779e'];
const HEAD='d0d1602d315de8f79b31b0656a9101b7a9cb8d9c';
const own={event_id:'EVT-GR-SORI-W02-1B-I-AUTH-REVIEW-RELEASE-20261019T084331Z',
  task_id:'SR-SORI-SM-W02.A-20261010',role:'guardrail',event_at_utc:'2026-10-19T08:43:31Z',kind:'RELEASE',version:12,lease_state:'RELEASED',
  repo_pr:'HeaInSeo/sori#42',head_or_digest:HEAD+' base 12bc3ea2 tree 8cb217ad'};
const heads=['## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3',
  '## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED',
  '## CURRENT CENTRAL OBSERVATION · sori_index','## CURRENT CENTRAL OBSERVATION · guardrail_queue',
  '## CURRENT INCIDENT MAINTENANCE · stale test'];
function raw(i,body,props={title:'Private test'},revision='2026-10-19T08:44:19.258Z'){
  return {structuredContent:{id:IDS[i],text:'<properties>\n'+JSON.stringify(props)+'\n</properties>\n<content>\n'+body+'\n</content>',page_last_edited_at:revision}};
}
function body(i,literal){return heads[i]+'\n'+literal+'\n## OWN WORKER PROOF\n'+
  (i===1?'```json\nQUEUE_EVENT_V1 '+JSON.stringify(own)+'\n```\n':'')+'immutable-tail-'+i;}
function loadHelper(f,overrides={}){
  let source=fs.readFileSync(path.join(__dirname,'reconcile_router_stale_prewrite.cjs'),'utf8');
  for(const [name,value] of Object.entries({ROOT:f.root,RUNTIME:f.runtime,PLAN:hash(JSON.stringify(f.edits)),
    ROUTER_BODY:hash(pageData(f.raw[4],IDS[4]).body),ROUTER_PROPS:hash(pageData(f.raw[4],IDS[4]).properties),
    ROUTER_REVISION:pageData(f.raw[4],IDS[4]).revision}))
    source=source.replace(new RegExp("const "+name+"='[^']*';"),'const '+name+'='+JSON.stringify(value)+';');
  source=source.replace('804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b',
    hash(fs.readFileSync(path.join(f.runtime,'policy.json'),'utf8')));
  const module={exports:{}};
  const req=name=>name==='./broker.cjs'?{...guard,...overrides}:require(name);
  vm.runInNewContext(source,{module,require:req,__dirname:f.runtime,process,console,Buffer},{filename:'reviewed-stale-helper.cjs'});
  return module.exports;
}
function fixture(t){
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'npg-stale-')),root=path.join(dir,'state'),runtime=path.join(dir,'runtime');
  fs.mkdirSync(runtime,{mode:0o700});
  const policy={productionEnabled:true,productionPages:IDS.slice(2),canaryPages:[],
    headingPrefixes:['## CURRENT CENTRAL OBSERVATION','## CURRENT INCIDENT MAINTENANCE'],
    centralSections:IDS.slice(0,2).map((pageId,i)=>({pageId,heading:heads[i]}))};
  for(const name of ['broker.cjs','runner.js'])fs.copyFileSync(path.join(__dirname,name),path.join(runtime,name));
  fs.writeFileSync(path.join(runtime,'policy.json'),JSON.stringify(policy),{mode:0o600});
  const b=new Broker(root,policy);
  t.after(()=>{b.close();fs.rmSync(dir,{recursive:true,force:true});});
  const edits=IDS.map((pageId,i)=>({pageId,heading:heads[i],oldStr:'central_status=WAIT_'+i,newStr:'central_status=REQUESTED_'+i}));
  b.handle({action:'begin',batchId:BATCH,edits});
  const pages=[];
  for(let i=0;i<4;i++){
    const before=raw(i,body(i,edits[i].oldStr));
    const receipt=b.handle({action:'prepare',batchId:BATCH,page:before});
    b.handle({action:'authorize',batchId:BATCH,ticket:receipt.ticket,page:before});
    pages.push(raw(i,body(i,edits[i].newStr)));
    b.handle({action:'verify',batchId:BATCH,ticket:receipt.ticket,page:pages[i]});
  }
  pages.push(raw(4,body(4,'central_status=FRESH_EXTRA_CONTEXT')));
  assert.throws(()=>b.handle({action:'prepare',batchId:BATCH,page:pages[4]}),e=>e.code==='AMBIGUOUS_LITERAL');
  b.handle({action:'halt',batchId:BATCH,reason:ERROR});
  const f={dir,root,runtime,b,policy,edits,raw:pages};f.helper=loadHelper(f);return f;
}
function assertUnchanged(f,action){
  const journal=fs.readFileSync(f.b.file,'utf8');
  assert.throws(action);
  assert.equal(fs.readFileSync(f.b.file,'utf8'),journal);
  assert.equal(fs.existsSync(path.join(f.root,hash(BATCH)+'.router-stale-prewrite-closure.json')),false);
}
test('stale final preflight remains blocked by ordinary runner admission and every normal write gate',t=>{
  const f=fixture(t),before=fs.readFileSync(f.b.file,'utf8');
  assert.equal(f.b.handle({action:'begin',batchId:BATCH,edits:f.edits}).status,'HALTED');
  assert.throws(()=>f.b.handle({action:'prepare',batchId:BATCH,page:f.raw[4]}));
  assert.throws(()=>f.b.handle({action:'begin',batchId:'bypass',edits:[f.edits[4]]}));
  assert.equal(fs.readFileSync(f.b.file,'utf8'),before);
});
test('explicit obsolete-final closure records four real receipts, zero writes/ACKs/advances and keeps immutable plan',t=>{
  const f=fixture(t),before=JSON.parse(fs.readFileSync(f.b.file)),fileHashes=new Map();
  for(const n of fs.readdirSync(f.root))if(n.startsWith(hash(BATCH)+'-'))fileHashes.set(n,hash(fs.readFileSync(path.join(f.root,n),'utf8')));
  const out=f.helper.apply(f.b,f.raw,false),after=JSON.parse(fs.readFileSync(f.b.file));
  assert.equal(out.status,'ABORTED_UNWRITTEN');assert.equal(after.active,null);
  assert.equal(after.batches[BATCH].next,4);assert.equal(after.batches[BATCH].receipt,null);
  assert.equal(after.batches[BATCH].planHash,before.batches[BATCH].planHash);
  assert.equal(JSON.stringify(after.batches[BATCH].edits),JSON.stringify(before.batches[BATCH].edits));
  for(const key of ['notionWrites','acknowledgedWrites','pageAdvances'])assert.equal(out.audit[key],0);
  assert.equal(out.audit.priorVerified,4);assert.equal(out.audit.unwrittenFinalEdits,1);
  for(const [n,h] of fileHashes)assert.equal(hash(fs.readFileSync(path.join(f.root,n),'utf8')),h);
  const evidence=JSON.parse(fs.readFileSync(out.auditFile));
  assert.equal(JSON.stringify(evidence.journalBefore),JSON.stringify(before));assert.equal(evidence.freshPages.length,5);
  assert.equal(fs.statSync(out.auditFile).mode&0o077,0);
  assert.equal(f.b.handle({action:'begin',batchId:BATCH,edits:f.edits}).status,'ABORTED_UNWRITTEN');
  assert.throws(()=>f.b.handle({action:'prepare',batchId:BATCH,page:f.raw[4]}));
  assert.equal(f.b.handle({action:'begin',batchId:'fresh',edits:[{...f.edits[4],oldStr:'central_status=FRESH_EXTRA_CONTEXT'}]}).status,'READY');
});
test('check-only is read-only and is the exported default',t=>{
  const f=fixture(t),before=fs.readFileSync(f.b.file,'utf8');
  assert.equal(f.helper.apply(f.b,f.raw).checkOnly,true);
  assert.equal(fs.readFileSync(f.b.file,'utf8'),before);assert.equal(f.b.state.active,BATCH);
  assert.equal(fs.existsSync(path.join(f.root,hash(BATCH)+'.router-stale-prewrite-closure.json')),false);
});
test('later genuine worker body and property records on a completed page survive read-only receipt validation',t=>{
  const f=fixture(t);
  f.raw[0]=raw(0,body(0,f.edits[0].newStr)+'\nnew own release preserved',{title:'Private test',version:19},'later');
  f.helper.apply(f.b,f.raw,false);
  const saved=JSON.parse(fs.readFileSync(path.join(f.root,hash(BATCH)+'.router-stale-prewrite-closure.json')));
  assert.equal(saved.freshPages[0].structuredContent.text,f.raw[0].structuredContent.text);
});
for(const [name,mutate] of [
  ['wrong active batch',f=>{f.b.state.active='other';f.b.save();}],
  ['wrong status',f=>{f.b.state.batches[BATCH].status='READY';f.b.save();}],
  ['wrong error',f=>{f.b.state.batches[BATCH].error='READBACK_MISMATCH';f.b.save();}],
  ['wrong next index',f=>{f.b.state.batches[BATCH].next=3;f.b.save();}],
  ['pending ticket',f=>{f.b.state.batches[BATCH].receipt={ticket:'unknown'};f.b.save();}],
  ['changed plan',f=>{const b=f.b.state.batches[BATCH];b.edits[4].newStr='changed';b.planHash=hash(JSON.stringify(b.edits));f.b.save();}],
  ['production hold',f=>{f.policy.productionEnabled=false;}],
  ['revoked exact central scope',f=>{f.policy.centralSections=[];}],
  ['missing fresh page',f=>{f.raw.pop();}],
  ['unordered fresh pages',f=>{[f.raw[0],f.raw[1]]=[f.raw[1],f.raw[0]];}],
  ['unknown fresh body',f=>{f.raw[0]=raw(0,body(0,f.edits[0].newStr)+'\n<unknown/>');}],
  ['changed reviewed Router body',f=>{f.raw[4]=raw(4,body(4,'another concurrent value'));}],
  ['changed reviewed Router properties',f=>{f.raw[4]=raw(4,body(4,'central_status=FRESH_EXTRA_CONTEXT'),{title:'changed'});}],
  ['changed reviewed Router revision',f=>{f.raw[4]=raw(4,body(4,'central_status=FRESH_EXTRA_CONTEXT'),{title:'Private test'},'later');}],
  ['missing latest successor RELEASE',f=>{f.raw[1]=raw(1,body(1,f.edits[1].newStr).replace(own.event_id,'old-event'));}],
  ['different successor head',f=>{f.raw[1]=raw(1,body(1,f.edits[1].newStr).replace(HEAD,'e'.repeat(40)));}],
  ['different successor role',f=>{f.raw[1]=raw(1,body(1,f.edits[1].newStr).replace('"role":"guardrail"','"role":"operator"'));}],
  ['completed central projection changed',f=>{f.raw[2]=raw(2,body(2,'central_status=other'));}],
  ['prepared final-page snapshot',f=>{atomic(f.b.snapshot(BATCH,4),{ticket:'exists'});}],
  ['unexpected old readback mismatch',f=>{atomic(f.b.snapshot(BATCH,0)+'.actual.json',{error:'old mismatch'});}],
  ['missing prior verified receipt',f=>{fs.unlinkSync(f.b.snapshot(BATCH,2)+'.verified.json');}],
  ['tampered prior protected tail',f=>{const p=f.b.snapshot(BATCH,0)+'.verified.json',v=JSON.parse(fs.readFileSync(p));v.page.body+='tampered';atomic(p,v);}],
  ['tampered preimage',f=>{const p=f.b.snapshot(BATCH,1),v=JSON.parse(fs.readFileSync(p));v.page.body+='tampered';atomic(p,v);}],
  ['changed runtime broker',f=>{fs.appendFileSync(path.join(f.runtime,'broker.cjs'),'\nchanged');}],
  ['changed runtime policy',f=>{fs.appendFileSync(path.join(f.runtime,'policy.json'),'\nchanged');}],
  ['changed runtime runner',f=>{fs.appendFileSync(path.join(f.runtime,'runner.js'),'\nchanged');}],
  ['journal changed outside lock',f=>{const j=JSON.parse(fs.readFileSync(f.b.file));j.version=99;atomic(f.b.file,j);}]
])test('one-incident stale closure rejects '+name+' without resolving the barrier',t=>{
  const f=fixture(t);mutate(f);assertUnchanged(f,()=>f.helper.apply(f.b,f.raw,false));
});
test('failed evidence persistence cannot clear the canonical barrier',t=>{
  const f=fixture(t),helper=loadHelper(f,{atomic(){throw new Error('disk full');}});
  assertUnchanged(f,()=>helper.apply(f.b,f.raw,false));assert.equal(f.b.state.active,BATCH);
});
test('replaying the explicit closure cannot acknowledge or advance an original failed page',t=>{
  const f=fixture(t);f.helper.apply(f.b,f.raw,false);const before=fs.readFileSync(f.b.file,'utf8');
  assert.throws(()=>f.helper.apply(f.b,f.raw,false));
  assert.equal(fs.readFileSync(f.b.file,'utf8'),before);assert.equal(f.b.state.batches[BATCH].next,4);
});
