'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const factory=vm.runInNewContext(fs.readFileSync(path.join(__dirname,'complete_projection.js'),'utf8'));
const shared=['3b286e6dd1d1817aa093fca807659f07','3b286e6dd1d181f2a75cfbd3885071c4',
  '3bf86e6dd1d18110938bc358e17c779e','3ed86e6dd1d18118b0b8d98bb91892fb','3b286e6dd1d1811d9308d9a2b56c9de4'];
const nk=['3f386e6dd1d18110bf66c5204284ed46','3f386e6dd1d181b5b467e20fafa69365'];
const sw=['3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7'];
const ni='3f086e6dd1d1818e9aecdc093a08cda9',si='3f586e6dd1d181ddb9f3e7217f64bfcd';
const marker='observation_id=`SYNC-TEST-1`';
const headings={
  [ni]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · nodekit_index',
  [si]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · sori_index',
  [sw[0]]:'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3',
  [sw[1]]:'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED',
  [shared[0]]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · operator_queue',
  [shared[1]]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · guardrail_queue',
  [shared[2]]:'## CURRENT INCIDENT MAINTENANCE · BODY_RANGE_OVERWRITE-20261011T2334Z · RESTORED_VERIFIED / ADOPTION_PENDING',
  [shared[3]]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · hub',
  [shared[4]]:'## CURRENT CENTRAL OBSERVATION · NPR-20261012 · dashboard'};
function fixture(repositories=['nodekit','sori'],fault){
  const ids=repositories.flatMap(r=>r==='nodekit'?nk:sw),bodies=repositories.flatMap(r=>r==='nodekit'?[ni]:[...sw,si]).concat(shared);
  const properties={'작업 상태':'검증대기','다음 담당자':'설계 에이전트','큐 구획':'조건 대기',
    '차단 대상':'existing owner trigger','재개 지시':'existing exact next action','큐 정리 근거':marker+' existing source'};
  const bundle={observationId:'SYNC-TEST-1',repositories,
    native:ids.map(pageId=>({pageId,properties:structuredClone(properties)})),
    edits:bodies.map(pageId=>({pageId,heading:headings[pageId],oldStr:'status=OLD-'+pageId,newStr:marker+'; status=NEW-'+pageId}))};
  const pages=new Map([...new Set([...ids,...bodies])].map(id=>[id,{id,props:{...properties,'이벤트 ID':'genuine-release','세션 ID':'own-session'},
    body:(headings[id]??'## Native')+'\nstatus=OLD-'+id+'\n## OWN WORKER\nimmutable proof',revision:'before'}]));
  let calls=0,reads=0;
  const tools={async mcp__codex_apps__notion_fetch({id}){
    reads++;const p=pages.get(id);
    const value={id,text:'<properties>\n'+JSON.stringify(p.props)+'\n</properties>\n<content>\n'+p.body+'\n</content>',page_last_edited_at:p.revision};
    if(fault==='incomplete')value.unknown_block_count=1;
    return {structuredContent:value};
  }};
  const config={createRunner(){return {async run(id,edits){
    calls++;
    for(const e of edits){const p=pages.get(e.pageId);p.body=p.body.replace(e.oldStr,e.newStr);p.revision='after';}
    if(fault==='lost-ack')throw new Error('UNCERTAIN_WRITE');
    if(fault==='new-own-event')pages.get(ids[0]).props['이벤트 ID']='genuine-new-claim';
    if(fault==='changed-own-body')pages.get(ids[0]).body+='\nnew own release';
    if(fault==='missing-final')pages.get(shared[4]).body='concurrently changed dashboard';
    if(fault==='bad-status')return {status:'HALTED'};
    return {status:'COMPLETED'};
  }};},runnerConfig:{}};
  return {bundle,pages,client:factory(tools,config),get calls(){return calls;},get reads(){return reads;}};
}
for(const repos of [['nodekit'],['sori'],['nodekit','sori']])test('mandatory manifest completes with full native/body readback: '+repos,async()=>{
  const f=fixture(repos),r=await f.client.run(f.bundle);
  assert.equal(r.status,'COMPLETE');assert.equal(r.bodyTargets,repos.length===2?9:repos[0]==='nodekit'?6:8);
  assert.equal(r.nativeTargets,repos.length*2);assert.equal(r.protectedContent,'UNCHANGED');assert.equal(f.calls,1);
  for(const p of f.pages.values())assert.equal(p.props['이벤트 ID'],'genuine-release');
});
for(const id of [ni,si,...sw,...shared])test('omitting mandatory projection '+id+' cannot start the body runner',async()=>{
  const f=fixture();f.bundle.edits=f.bundle.edits.filter(e=>e.pageId!==id);
  await assert.rejects(f.client.run(f.bundle),/mandatory body/);assert.equal(f.calls,0);assert.equal(f.reads,0);
});
for(const id of [...nk,...sw])test('omitting native metadata '+id+' cannot start the runner',async()=>{
  const f=fixture();f.bundle.native=f.bundle.native.filter(e=>e.pageId!==id);
  await assert.rejects(f.client.run(f.bundle),/native metadata/);assert.equal(f.calls,0);
});
for(const [name,mutate] of [
  ['wrong heading',b=>{b.edits[0].heading='## OWN WORKER';}],
  ['missing body observation',b=>{b.edits[0].newStr='status=NEW';}],
  ['missing metadata observation',b=>{b.native[0].properties['큐 정리 근거']='old source';}],
  ['missing next action',b=>{delete b.native[0].properties['재개 지시'];}],
  ['worker proof property',b=>{b.native[0].properties['이벤트 ID']='fake';}],
  ['duplicate repo',b=>{b.repositories=['nodekit','nodekit'];}],
  ['unknown repo',b=>{b.repositories=['nodevault'];}],
  ['duplicate body',b=>{b.edits[0]=b.edits[1];}],
  ['wrong native page',b=>{b.native[0].pageId='f'.repeat(32);}]
])test('reject invalid complete manifest: '+name,async()=>{
  const f=fixture();mutate(f.bundle);await assert.rejects(f.client.run(f.bundle));assert.equal(f.calls,0);
});
test('drifting native current metadata prevents any body mutation',async()=>{
  const f=fixture();f.pages.get(nk[0]).props['차단 대상']='old long-running Codex';
  await assert.rejects(f.client.run(f.bundle),/Native property differs/);assert.equal(f.calls,0);
});
for(const fault of ['incomplete','lost-ack','new-own-event','changed-own-body','missing-final','bad-status'])
  test('no complete certificate after '+fault,async()=>{
    const f=fixture(undefined,fault);await assert.rejects(f.client.run(f.bundle));
    if(fault==='incomplete')assert.equal(f.calls,0);
    if(fault==='new-own-event')assert.equal(f.pages.get(nk[0]).props['이벤트 ID'],'genuine-new-claim');
  });
