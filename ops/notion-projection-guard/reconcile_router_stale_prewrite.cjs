#!/usr/bin/env node
'use strict';
// Explicit closure of one obsolete, unwritten final edit. No Notion request,
// ACK, page advance, plan amendment or production-policy change is provided.
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {Broker,hash,pageData,plan,atomic}=require('./broker.cjs');
const ROOT='/home/heain/.local/state/notion-projection-guard';
const RUNTIME='/home/heain/.local/share/notion-projection-guard';
const BATCH='QSYNC-20261019T082053Z-SORI42-RELEASE-ADMISSION';
const PLAN='437fa09117ad1859880968f330b31b2902826245e9c8b52bf46cbf89681a12d0';
const ERROR='AMBIGUOUS_LITERAL: Old literal must occur exactly once in the entire body';
const IDS=['3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7',
  '3f586e6dd1d181ddb9f3e7217f64bfcd','3b286e6dd1d181f2a75cfbd3885071c4',
  '3bf86e6dd1d18110938bc358e17c779e'];
const ROUTER_BODY='357636fbe4aea4f8b5f6cf492c7d8d496a70ae2853d41d39eb125eb66d8f8125';
const ROUTER_PROPS='e828f06c39d3c4e7c88c37b03e914e0a2121e62e1e1ebbce4e827564c40c8506';
const ROUTER_REVISION='2026-10-19T08:01:46.997Z';
const REVIEW_EVENT='EVT-GR-SORI-W02-1B-I-AUTH-REVIEW-RELEASE-20261019T084331Z';
const REVIEW_HEAD='d0d1602d315de8f79b31b0656a9101b7a9cb8d9c';
const PINS={'broker.cjs':'b0b07630eebb9d2ebbc8a4c85bd9cdded9ca2c5432a66a8cebd45e7d8f1f102c',
  'policy.json':'804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b',
  'runner.js':'278b3e544f9ba10538f73901c705262d9a3ef76232a931d67da6ed80750c6d52'};
function must(ok,message){if(!ok)throw new Error('ROUTER_STALE_RECONCILE: '+message);}
function read(file){
  const st=fs.lstatSync(file);
  must(st.isFile()&&!st.isSymbolicLink()&&!(st.mode&0o077),'Private regular evidence required');
  return JSON.parse(fs.readFileSync(file,'utf8'));
}
function pins(){
  for(const [name,expected] of Object.entries(PINS))
    must(hash(fs.readFileSync(path.join(RUNTIME,name),'utf8'))===expected,'Approved runtime changed: '+name);
  must(hash(fs.readFileSync(path.join(__dirname,'broker.cjs'),'utf8'))===PINS['broker.cjs'],
    'Helper broker differs from approved runtime');
}
function inspect(broker,raw){
  const b=broker.state.batches[BATCH];
  must(broker.state.active===BATCH&&b?.status==='HALTED'&&b.next===4&&b.receipt===null&&
    b.error===ERROR&&b.edits.length===5&&b.planHash===PLAN&&hash(JSON.stringify(b.edits))===PLAN,
    'Exact five-page pre-prepare incident required');
  must(JSON.stringify(read(broker.file))===JSON.stringify(broker.state),'Canonical journal changed outside its lock');
  must(b.edits.every((e,i)=>e.pageId===IDS[i]),'Original ordered scope changed');
  const admission=broker.handle({action:'begin',batchId:BATCH,edits:b.edits});
  must(admission.status==='HALTED'&&admission.next===4&&admission.reused===true,'Original policy admission changed');
  const prefix=hash(BATCH)+'-',allowed=new Set();
  for(let i=0;i<4;i++){allowed.add(prefix+i+'.json');allowed.add(prefix+i+'.json.verified.json');}
  for(const name of fs.readdirSync(broker.root))
    if(name.startsWith(prefix))must(allowed.has(name),'Pending or unexpected prepared evidence exists');
  must(Array.isArray(raw)&&raw.length===5,'Five fresh complete ordered page reads required');
  const pages=raw.map((value,i)=>pageData(value,IDS[i])),receipts=[];
  must(pages.every(p=>typeof p.revision==='string'&&p.revision.length>0),'Fresh revisions required');
  for(let i=0;i<4;i++){
    const edit=b.edits[i],file=broker.snapshot(BATCH,i),snap=read(file),verified=read(file+'.verified.json');
    must(snap.batchId===BATCH&&snap.index===i&&typeof snap.ticket==='string'&&snap.ticket.length>0&&
      JSON.stringify(snap.edit)===JSON.stringify(edit)&&hash(snap.page.body)===snap.bodyHash&&
      hash(snap.page.properties)===snap.propertiesHash&&hash(snap.expected)===snap.expectedHash&&
      !verified.reconciliation,'Original completed snapshot changed');
    const expected=plan(snap.page,edit),before=pageData(snap.page.raw,IDS[i]),saved=pageData(verified.page.raw,IDS[i]);
    must(expected.expected===snap.expected&&expected.beforeHash===snap.beforeHash&&expected.afterHash===snap.afterHash&&
      before.body===snap.page.body&&before.properties===snap.page.properties&&before.revision===snap.page.revision&&
      saved.body===snap.expected&&saved.body===verified.page.body&&verified.bodyHash===snap.expectedHash&&
      saved.properties===snap.page.properties&&saved.properties===verified.page.properties&&
      saved.revision===verified.page.revision,'Original full protected-content receipt changed');
    // Later real worker records are retained. No completed page is rewritten.
    // Its central literal must still be unique and inside its original scope.
    plan(pages[i],{...edit,oldStr:edit.newStr,newStr:'central_recovery_scope_check='+i+';'+hash(BATCH)});
    receipts.push({index:i,pageId:IDS[i],snapshotHash:hash(fs.readFileSync(file,'utf8')),
      verifiedHash:hash(fs.readFileSync(file+'.verified.json','utf8')),
      originalExpectedHash:snap.expectedHash,freshBodyHash:hash(pages[i].body),
      freshPropertiesHash:hash(pages[i].properties),freshRevision:pages[i].revision});
  }
  const router=pages[4],edit=b.edits[4];
  must(hash(router.body)===ROUTER_BODY&&hash(router.properties)===ROUTER_PROPS&&router.revision===ROUTER_REVISION,
    'Reviewed stale Router preimage changed');
  must(!router.body.includes(edit.oldStr)&&!router.body.includes(edit.newStr),
    'Only absent old literal and wholly unwritten output are admitted');
  let missing=false;
  try{plan(router,edit);}catch(e){missing=e.code==='AMBIGUOUS_LITERAL';}
  must(missing,'Original final edit must fail before prepare');
  const eventText=pages[1].body.match(/QUEUE_EVENT_V1\s+(\{[^\n]+\})/);
  const event=eventText?JSON.parse(eventText[1]):null;
  must(event?.event_id===REVIEW_EVENT&&event.task_id==='SR-SORI-SM-W02.A-20261010'&&
    event.role==='guardrail'&&event.event_at_utc==='2026-10-19T08:43:31Z'&&
    event.kind==='RELEASE'&&event.version===12&&event.lease_state==='RELEASED'&&event.repo_pr==='HeaInSeo/sori#42'&&
    event.head_or_digest?.startsWith(REVIEW_HEAD+' base '),
    'Existing successor own RELEASE must supersede the obsolete review request');
  return {kind:'REVIEWED_UNWRITTEN_STALE_ROUTER_CLOSURE',batchId:BATCH,planHash:PLAN,error:ERROR,
    next:4,priorVerified:4,unwrittenFinalEdits:1,notionWrites:0,acknowledgedWrites:0,pageAdvances:0,
    routerBodyHash:ROUTER_BODY,routerPropertiesHash:ROUTER_PROPS,routerRevision:ROUTER_REVISION,
    successorEvent:REVIEW_EVENT,successorHead:REVIEW_HEAD,receipts};
}
function apply(broker,raw,checkOnly=true){
  pins();must(broker.root===ROOT,'Canonical production state required');
  const journalBefore=fs.readFileSync(broker.file,'utf8'),audit=inspect(broker,raw);
  must(fs.readFileSync(broker.file,'utf8')===journalBefore,'Canonical journal changed during inspection');
  if(checkOnly)return {checkOnly:true,audit};
  const auditFile=path.join(ROOT,hash(BATCH)+'.router-stale-prewrite-closure.json');
  must(!fs.existsSync(auditFile),'Existing closure audit requires separate reconciliation');
  audit.at=new Date().toISOString();audit.journalBeforeHash=hash(journalBefore);
  const expected=JSON.parse(journalBefore),b=expected.batches[BATCH];
  b.status='ABORTED_UNWRITTEN';b.resolution=audit;expected.active=null;
  // Save complete evidence before resolving the barrier. The original plan,
  // next=4, error, receipt and all other batches remain exactly preserved.
  atomic(auditFile,{audit,journalBefore:JSON.parse(journalBefore),freshPages:raw});
  must(fs.readFileSync(broker.file,'utf8')===journalBefore,'Journal changed before explicit closure');
  atomic(broker.file,expected);
  const after=read(broker.file);
  must(JSON.stringify(after)===JSON.stringify(expected),'Closure readback differs');
  broker.state=after;
  return {checkOnly:false,status:b.status,next:b.next,active:null,audit,auditFile};
}
if(require.main===module){
  let broker;
  try{
    must(process.argv.slice(2).every(a=>['--check-only','--apply'].includes(a))&&
      !(process.argv.includes('--check-only')&&process.argv.includes('--apply')),'Choose check-only or explicit apply');
    pins();broker=new Broker(ROOT,JSON.parse(fs.readFileSync(path.join(RUNTIME,'policy.json'),'utf8')));
  }catch(e){process.stdout.write('NPG-RS1 '+JSON.stringify({ok:false,error:e.message})+'\n');process.exit(1);}
  let closed=false;function close(){if(!closed){closed=true;broker.close();}}
  process.on('exit',close);for(const signal of ['SIGINT','SIGTERM','SIGHUP'])process.on(signal,()=>process.exit(1));
  const rl=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
  process.stdout.write('NPG-RS1 {"ready":true}\n');
  rl.once('line',line=>{
    try{
      const result=apply(broker,JSON.parse(line),!process.argv.includes('--apply'));
      process.stdout.write('NPG-RS1 '+JSON.stringify({ok:true,...result})+'\n',()=>process.exit(0));
    }catch(e){process.stdout.write('NPG-RS1 '+JSON.stringify({ok:false,error:e.message})+'\n',()=>process.exit(1));}
  });
  rl.on('close',close);
}
module.exports={inspect,apply};
