#!/usr/bin/env node
'use strict';
// Explicit one-incident recovery. No broker action, Notion request or ACK is added.
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {Broker,hash,pageData,plan,atomic}=require('./broker.cjs');
const ROOT='/home/heain/.local/state/notion-projection-guard';
const RUNTIME='/home/heain/.local/share/notion-projection-guard';
const BATCH='QSYNC-20261019T0245Z-SORI40-LANDED-RWC1-ACCEPTED-NK138-CODEX-RUNNING';
const PLAN='9bf293d4799801a673b5c9373cdaa2dbd28c6c31a0c18b17bdd46a707fc04044';
const ERROR='REMOTE_ERROR: McpServerError: Connection failed.';
const PINS={'broker.cjs':'b0b07630eebb9d2ebbc8a4c85bd9cdded9ca2c5432a66a8cebd45e7d8f1f102c',
  'policy.json':'804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b',
  'runner.js':'278b3e544f9ba10538f73901c705262d9a3ef76232a931d67da6ed80750c6d52'};
function must(ok,message){if(!ok)throw new Error('PREFLIGHT_RECONCILE: '+message);}
function read(file){
  const st=fs.lstatSync(file);
  must(st.isFile()&&!st.isSymbolicLink()&&!(st.mode&0o077),'Private regular evidence required');
  return JSON.parse(fs.readFileSync(file,'utf8'));
}
function pins(){
  for(const [name,expected] of Object.entries(PINS))
    must(hash(fs.readFileSync(path.join(RUNTIME,name),'utf8'))===expected,'Approved runtime changed: '+name);
  must(hash(fs.readFileSync(path.join(__dirname,'broker.cjs'),'utf8'))===PINS['broker.cjs'],'Helper broker differs from approved runtime');
}
function inspect(broker,raw){
  const b=broker.state.batches[BATCH];
  must(broker.state.active===BATCH&&b?.status==='HALTED'&&b.next===1&&b.receipt===null&&
    b.error===ERROR&&b.edits.length===9&&b.planHash===PLAN&&hash(JSON.stringify(b.edits))===PLAN,
    'Exact halted pre-prepare connection incident required');
  must(JSON.stringify(read(broker.file))===JSON.stringify(broker.state),'Canonical journal changed outside its lock');
  const admission=broker.handle({action:'begin',batchId:BATCH,edits:b.edits});
  must(admission.status==='HALTED'&&admission.next===1&&admission.reused===true,'Original policy admission changed');
  const prefix=hash(BATCH)+'-';
  for(const name of fs.readdirSync(broker.root)){
    if(!name.startsWith(prefix))continue;
    must(name===prefix+'0.json'||name===prefix+'0.json.verified.json',
      'Pending or unexpected prepared evidence exists');
  }
  must(Array.isArray(raw)&&raw.length===9,'Nine fresh complete ordered page reads required');
  const pages=raw.map((value,i)=>pageData(value,b.edits[i].pageId));
  const edit=b.edits[0],file=broker.snapshot(BATCH,0),snap=read(file),verified=read(file+'.verified.json');
  must(snap.batchId===BATCH&&snap.index===0&&typeof snap.ticket==='string'&&
    JSON.stringify(snap.edit)===JSON.stringify(edit)&&hash(snap.page.body)===snap.bodyHash&&
    hash(snap.page.properties)===snap.propertiesHash&&hash(snap.expected)===snap.expectedHash&&
    plan(snap.page,edit).expected===snap.expected&&!verified.reconciliation,'Original completed snapshot changed');
  const before=pageData(snap.page.raw,edit.pageId),saved=pageData(verified.page.raw,edit.pageId);
  must(before.body===snap.page.body&&before.properties===snap.page.properties&&before.revision===snap.page.revision&&
    saved.body===snap.expected&&saved.body===verified.page.body&&verified.bodyHash===snap.expectedHash&&
    saved.properties===snap.page.properties&&saved.properties===verified.page.properties&&
    saved.revision===verified.page.revision,'Original protected-content receipt changed');
  must(pages[0].body===saved.body&&pages[0].properties===saved.properties&&pages[0].revision===saved.revision,
    'Completed page changed; never rewrite or roll back it');
  const fresh=pages.map((page,i)=>{
    must(typeof page.revision==='string'&&page.revision.length>0,'Fresh revision required');
    if(i>0)plan(page,b.edits[i]); // Unique old literal, absent new literal, same bounded central section.
    return {index:i,pageId:b.edits[i].pageId,bodyHash:hash(page.body),
      propertiesHash:hash(page.properties),revision:page.revision};
  });
  return {kind:'REVIEWED_SINGLE_PREFLIGHT_CONNECTION_FAILURE',batchId:BATCH,planHash:PLAN,
    error:ERROR,next:1,ticket:null,priorVerified:1,notionWrites:0,acknowledgedWrites:0,pageAdvances:0,
    snapshotHash:hash(JSON.stringify(snap)),verifiedHash:hash(JSON.stringify(verified)),fresh};
}
function apply(broker,raw,checkOnly){
  pins();
  const journalBefore=fs.readFileSync(broker.file,'utf8'),audit=inspect(broker,raw);
  must(fs.readFileSync(broker.file,'utf8')===journalBefore,'Canonical journal changed during inspection');
  if(checkOnly)return {checkOnly:true,audit};
  const auditFile=path.join(ROOT,hash(BATCH)+'.preflight-connection-reconciliation.json');
  must(!fs.existsSync(auditFile),'Existing recovery audit requires separate reconciliation');
  audit.at=new Date().toISOString();
  atomic(auditFile,{audit,freshPages:raw});
  must(fs.readFileSync(broker.file,'utf8')===journalBefore,'Journal changed before explicit recovery');
  const b=broker.state.batches[BATCH];
  b.reconciliations=[...(b.reconciliations??[]),audit];b.status='READY';broker.save();
  const after=read(broker.file);
  must(after.active===BATCH&&after.batches[BATCH].status==='READY'&&
    after.batches[BATCH].next===1&&after.batches[BATCH].receipt===null&&
    after.batches[BATCH].planHash===PLAN&&hash(JSON.stringify(after.batches[BATCH].edits))===PLAN,
    'Recovery readback differs');
  return {checkOnly:false,status:'READY',next:1,audit,auditFile};
}
if(require.main===module){
  let broker;
  try{
    pins();
    broker=new Broker(ROOT,JSON.parse(fs.readFileSync(path.join(RUNTIME,'policy.json'),'utf8')));
  }catch(e){process.stdout.write('NPG-PC1 '+JSON.stringify({ok:false,error:e.message})+'\n');process.exit(1);}
  let closed=false;
  function close(){if(!closed){closed=true;broker.close();}}
  process.on('exit',close);
  for(const signal of ['SIGINT','SIGTERM','SIGHUP'])process.on(signal,()=>process.exit(1));
  const rl=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
  process.stdout.write('NPG-PC1 {"ready":true}\n');
  rl.once('line',line=>{
    try{
      const result=apply(broker,JSON.parse(line),process.argv.includes('--check-only'));
      process.stdout.write('NPG-PC1 '+JSON.stringify({ok:true,...result})+'\n',()=>process.exit(0));
    }catch(e){process.stdout.write('NPG-PC1 '+JSON.stringify({ok:false,error:e.message})+'\n',()=>process.exit(1));}
  });
  rl.on('close',close);
}
module.exports={inspect,apply};
