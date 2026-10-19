'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),vm=require('node:vm');
const guard=require('./broker.cjs'),{Broker,hash,atomic}=guard;
const BATCH='QSYNC-20261019T132702Z-NK138-LANDED-SORI43-RECIRCULATE';
const ERROR='REMOTE_ERROR: tool call error: tool call failed for `codex_apps/remote_desktop_commander.read_process_output`\n\nCaused by:\n    Mcp error: -32603: Internal error';
const IDS=['3f086e6dd1d1818e9aecdc093a08cda9','3f586e6dd1d181ebb43dd7de62aa2ca4',
  '3f586e6dd1d181cd9f5bdcdab6290de7','3f586e6dd1d181ddb9f3e7217f64bfcd',
  '3b286e6dd1d1817aa093fca807659f07','3b286e6dd1d181f2a75cfbd3885071c4','3bf86e6dd1d18110938bc358e17c779e'];
const heads=IDS.map((_,i)=>i===1?'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3':
  i===2?'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED':'## CURRENT CENTRAL OBSERVATION · test'+i);
function raw(i,literal,props={title:'Private test'},revision='fresh'){
  return {structuredContent:{id:IDS[i],text:'<properties>\n'+JSON.stringify(props)+'\n</properties>\n<content>\n'+
    heads[i]+'\n'+literal+'\n## OWN WORKER PROOF\nimmutable-tail-'+i+'\n</content>',page_last_edited_at:revision}};
}
function helper(f,overrides={}){
  let source=fs.readFileSync(path.join(__dirname,'reconcile_transport_preprepare.cjs'),'utf8');
  for(const [name,value] of Object.entries({ROOT:f.root,RUNTIME:f.runtime,PLAN:hash(JSON.stringify(f.edits)),
    SNAPSHOT_HASH:hash(fs.readFileSync(f.b.snapshot(BATCH,0),'utf8')),
    VERIFIED_HASH:hash(fs.readFileSync(f.b.snapshot(BATCH,0)+'.verified.json','utf8'))}))
    source=source.replace(new RegExp("const "+name+"='[^']*';"),'const '+name+'='+JSON.stringify(value)+';');
  source=source.replace('804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b',hash(fs.readFileSync(path.join(f.runtime,'policy.json'),'utf8')));
  const module={exports:{}};
  vm.runInNewContext(source,{module,require:n=>n==='./broker.cjs'?{...guard,...overrides}:require(n),__dirname:f.runtime,process,Buffer});
  return module.exports;
}
function fixture(t){
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'npg-transport-')),root=path.join(dir,'state'),runtime=path.join(dir,'runtime');
  fs.mkdirSync(runtime,{mode:0o700});
  const policy={productionEnabled:true,productionPages:IDS.filter((_,i)=>i!==1&&i!==2),canaryPages:[],
    headingPrefixes:['## CURRENT CENTRAL OBSERVATION'],centralSections:[1,2].map(i=>({pageId:IDS[i],heading:heads[i]}))};
  for(const n of ['broker.cjs','runner.js'])fs.copyFileSync(path.join(__dirname,n),path.join(runtime,n));
  fs.writeFileSync(path.join(runtime,'policy.json'),JSON.stringify(policy),{mode:0o600});
  const b=new Broker(root,policy);t.after(()=>{b.close();fs.rmSync(dir,{recursive:true,force:true});});
  const edits=IDS.map((pageId,i)=>({pageId,heading:heads[i],oldStr:'central=OLD_'+i,newStr:'central=NEW_'+i}));
  b.handle({action:'begin',batchId:BATCH,edits});
  const before=raw(0,edits[0].oldStr),r=b.handle({action:'prepare',batchId:BATCH,page:before});
  b.handle({action:'authorize',batchId:BATCH,ticket:r.ticket,page:before});
  const pages=IDS.map((_,i)=>raw(i,i===0?edits[i].newStr:edits[i].oldStr));
  b.handle({action:'verify',batchId:BATCH,ticket:r.ticket,page:pages[0]});
  b.handle({action:'halt',batchId:BATCH,reason:ERROR});
  const f={dir,root,runtime,policy,b,edits,raw:pages};f.helper=helper(f);return f;
}
function unchanged(f,action){
  const before=fs.readFileSync(f.b.file,'utf8');assert.throws(action);
  assert.equal(fs.readFileSync(f.b.file,'utf8'),before);
  assert.equal(fs.existsSync(path.join(f.root,hash(BATCH)+'.transport-preprepare-closure.json')),false);
}
test('transport closure preserves the first verified receipt and immutable remaining plan, with zero writes or advances',t=>{
  const f=fixture(t),before=JSON.parse(fs.readFileSync(f.b.file)),snapshot=f.b.snapshot(BATCH,0),hashes=[snapshot,snapshot+'.verified.json'].map(p=>hash(fs.readFileSync(p,'utf8')));
  const out=f.helper.apply(f.b,f.raw,false),after=JSON.parse(fs.readFileSync(f.b.file));
  assert.equal(after.active,null);assert.equal(after.batches[BATCH].status,'ABORTED_UNWRITTEN');
  for(const k of ['next','receipt','planHash','edits','error'])assert.deepEqual(after.batches[BATCH][k],before.batches[BATCH][k]);
  for(const k of ['notionWrites','acknowledgedWrites','pageAdvances'])assert.equal(out.audit[k],0);
  assert.equal(out.audit.priorVerified,1);assert.equal(out.audit.unwrittenRemainingEdits,6);
  assert.deepEqual([snapshot,snapshot+'.verified.json'].map(p=>hash(fs.readFileSync(p,'utf8'))),hashes);
  assert.equal(fs.statSync(out.auditFile).mode&0o077,0);
  assert.equal(JSON.parse(fs.readFileSync(out.auditFile)).freshPages.length,7);
  assert.throws(()=>f.b.handle({action:'prepare',batchId:BATCH,page:f.raw[1]}));
  assert.equal(f.b.handle({action:'begin',batchId:'fresh-observation',edits:[f.edits[1]]}).status,'READY');
});
test('transport closure defaults to check-only and cannot be replayed',t=>{
  const f=fixture(t),before=fs.readFileSync(f.b.file,'utf8');assert.equal(f.helper.apply(f.b,f.raw).checkOnly,true);
  assert.equal(fs.readFileSync(f.b.file,'utf8'),before);
  f.helper.apply(f.b,f.raw,false);const after=fs.readFileSync(f.b.file,'utf8');
  assert.throws(()=>f.helper.apply(f.b,f.raw,false));assert.equal(fs.readFileSync(f.b.file,'utf8'),after);
});
for(const [name,mutate] of [
  ['other active',f=>{f.b.state.active='other';f.b.save();}],
  ['ready status',f=>{f.b.state.batches[BATCH].status='READY';f.b.save();}],
  ['different error',f=>{f.b.state.batches[BATCH].error='REMOTE_ERROR: other';f.b.save();}],
  ['advanced index',f=>{f.b.state.batches[BATCH].next=2;f.b.save();}],
  ['uncertain receipt',f=>{f.b.state.batches[BATCH].receipt={ticket:'pending'};f.b.save();}],
  ['modified plan',f=>{const b=f.b.state.batches[BATCH];b.edits[1].newStr='other';b.planHash=hash(JSON.stringify(b.edits));f.b.save();}],
  ['production hold',f=>{f.policy.productionEnabled=false;}],
  ['revoked section',f=>{f.policy.centralSections=[];}],
  ['prepared next page',f=>{atomic(f.b.snapshot(BATCH,1),{ticket:'possible-write'});}],
  ['unexpected actual receipt',f=>{atomic(f.b.snapshot(BATCH,0)+'.actual.json',{});}],
  ['missing verified receipt',f=>{fs.unlinkSync(f.b.snapshot(BATCH,0)+'.verified.json');}],
  ['deleted receipt timestamp',f=>{const p=f.b.snapshot(BATCH,0)+'.verified.json',s=JSON.parse(fs.readFileSync(p));delete s.at;atomic(p,s);}],
  ['replaced receipt timestamp',f=>{const p=f.b.snapshot(BATCH,0)+'.verified.json',s=JSON.parse(fs.readFileSync(p));s.at='different';atomic(p,s);}],
  ['tampered prior receipt',f=>{const p=f.b.snapshot(BATCH,0)+'.verified.json',s=JSON.parse(fs.readFileSync(p));s.page.body+='bad';atomic(p,s);}],
  ['tampered snapshot',f=>{const p=f.b.snapshot(BATCH,0),s=JSON.parse(fs.readFileSync(p));s.page.body+='bad';atomic(p,s);}],
  ['changed completed body',f=>{f.raw[0]=raw(0,f.edits[0].newStr+' changed');}],
  ['changed completed properties',f=>{f.raw[0]=raw(0,f.edits[0].newStr,{title:'other'});}],
  ['changed completed revision',f=>{f.raw[0]=raw(0,f.edits[0].newStr,{title:'Private test'},'later');}],
  ['already written remaining page',f=>{f.raw[1]=raw(1,f.edits[1].newStr);}],
  ['missing old literal',f=>{f.raw[2]=raw(2,'another value');}],
  ['incomplete source',f=>{f.raw[3]=raw(3,f.edits[3].oldStr+'<unknown/>');}],
  ['unordered source',f=>{[f.raw[1],f.raw[2]]=[f.raw[2],f.raw[1]];}],
  ['missing page',f=>{f.raw.pop();}],
  ['changed broker',f=>{fs.appendFileSync(path.join(f.runtime,'broker.cjs'),'changed');}],
  ['changed runner',f=>{fs.appendFileSync(path.join(f.runtime,'runner.js'),'changed');}],
  ['changed policy',f=>{fs.appendFileSync(path.join(f.runtime,'policy.json'),'changed');}],
  ['concurrent journal',f=>{const j=JSON.parse(fs.readFileSync(f.b.file));j.extra='outside-lock';atomic(f.b.file,j);}]
])test('transport closure rejects '+name+' without clearing the barrier',t=>{
  const f=fixture(t);mutate(f);unchanged(f,()=>f.helper.apply(f.b,f.raw,false));
});
test('failed private evidence write cannot release the canonical barrier',t=>{
  const f=fixture(t),h=helper(f,{atomic(){throw new Error('disk full');}});
  unchanged(f,()=>h.apply(f.b,f.raw,false));assert.equal(f.b.state.active,BATCH);
});
