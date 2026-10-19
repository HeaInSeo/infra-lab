#!/usr/bin/env node
'use strict';
// Explicit closure of one transport failure before page 1 was prepared.
// This never authorizes, writes, acknowledges or advances a Notion page.
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {Broker,hash,pageData,plan,atomic}=require('./broker.cjs');
const ROOT='/home/heain/.local/state/notion-projection-guard';
const RUNTIME='/home/heain/.local/share/notion-projection-guard';
const BATCH='QSYNC-20261019T132702Z-NK138-LANDED-SORI43-RECIRCULATE';
const PLAN='677ffcf80f9e3c7855b8a1a02f79ab7cf1513e1ce8649cb45f2919d901d2b065';
const ERROR='REMOTE_ERROR: tool call error: tool call failed for `codex_apps/remote_desktop_commander.read_process_output`\n\nCaused by:\n    Mcp error: -32603: Internal error';
const IDS=['3f086e6dd1d1818e9aecdc093a08cda9','3f586e6dd1d181ebb43dd7de62aa2ca4',
  '3f586e6dd1d181cd9f5bdcdab6290de7','3f586e6dd1d181ddb9f3e7217f64bfcd',
  '3b286e6dd1d1817aa093fca807659f07','3b286e6dd1d181f2a75cfbd3885071c4','3bf86e6dd1d18110938bc358e17c779e'];
const PINS={'broker.cjs':'b0b07630eebb9d2ebbc8a4c85bd9cdded9ca2c5432a66a8cebd45e7d8f1f102c',
  'policy.json':'804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b',
  'runner.js':'278b3e544f9ba10538f73901c705262d9a3ef76232a931d67da6ed80750c6d52'};
function must(ok,message){if(!ok)throw new Error('TRANSPORT_PREPREPARE: '+message);}
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
    b.error===ERROR&&b.edits.length===7&&b.planHash===PLAN&&hash(JSON.stringify(b.edits))===PLAN,
    'Exact seven-page transport incident required');
  must(JSON.stringify(read(broker.file))===JSON.stringify(broker.state),'Canonical journal changed outside its lock');
  must(b.edits.every((e,i)=>e.pageId===IDS[i]),'Original ordered scope changed');
  const admission=broker.handle({action:'begin',batchId:BATCH,edits:b.edits});
  must(admission.status==='HALTED'&&admission.next===1&&admission.reused===true,'Original policy admission changed');
  const prefix=hash(BATCH)+'-';
  for(const name of fs.readdirSync(broker.root))if(name.startsWith(prefix))
    must(name===prefix+'0.json'||name===prefix+'0.json.verified.json','Pending or unexpected prepared evidence exists');
  must(Array.isArray(raw)&&raw.length===7,'Seven fresh complete ordered page reads required');
  const pages=raw.map((value,i)=>pageData(value,IDS[i]));
  const edit=b.edits[0],file=broker.snapshot(BATCH,0),snap=read(file),verified=read(file+'.verified.json');
  must(snap.batchId===BATCH&&snap.index===0&&typeof snap.ticket==='string'&&snap.ticket.length>0&&
    JSON.stringify(snap.edit)===JSON.stringify(edit)&&hash(snap.page.body)===snap.bodyHash&&
    hash(snap.page.properties)===snap.propertiesHash&&hash(snap.expected)===snap.expectedHash&&
    !verified.reconciliation,'Original completed snapshot changed');
  const expected=plan(snap.page,edit),before=pageData(snap.page.raw,IDS[0]),saved=pageData(verified.page.raw,IDS[0]);
  must(expected.expected===snap.expected&&expected.beforeHash===snap.beforeHash&&expected.afterHash===snap.afterHash&&
    before.body===snap.page.body&&before.properties===snap.page.properties&&before.revision===snap.page.revision&&
    saved.body===snap.expected&&saved.body===verified.page.body&&verified.bodyHash===snap.expectedHash&&
    saved.properties===snap.page.properties&&saved.properties===verified.page.properties&&
    saved.revision===verified.page.revision,'Original full protected-content receipt changed');
  must(pages[0].body===saved.body&&pages[0].properties===saved.properties&&pages[0].revision===saved.revision,
    'Completed page changed; never rewrite or roll back it');
  const fresh=pages.map((page,i)=>{
    must(typeof page.revision==='string'&&page.revision.length>0,'Fresh revision required');
    if(i>0)plan(page,b.edits[i]); // All six outputs are absent and their old literals remain unique.
    return {index:i,pageId:IDS[i],bodyHash:hash(page.body),propertiesHash:hash(page.properties),revision:page.revision};
  });
  return {kind:'REVIEWED_UNWRITTEN_TRANSPORT_PREPREPARE_CLOSURE',batchId:BATCH,planHash:PLAN,
    error:ERROR,next:1,priorVerified:1,unwrittenRemainingEdits:6,notionWrites:0,acknowledgedWrites:0,pageAdvances:0,
    snapshotHash:hash(fs.readFileSync(file,'utf8')),verifiedHash:hash(fs.readFileSync(file+'.verified.json','utf8')),fresh};
}
function apply(broker,raw,checkOnly=true){
  pins();must(broker.root===ROOT,'Canonical production state required');
  const journalBefore=fs.readFileSync(broker.file,'utf8'),audit=inspect(broker,raw);
  must(fs.readFileSync(broker.file,'utf8')===journalBefore,'Canonical journal changed during inspection');
  if(checkOnly)return {checkOnly:true,audit};
  const auditFile=path.join(ROOT,hash(BATCH)+'.transport-preprepare-closure.json');
  must(!fs.existsSync(auditFile),'Existing closure audit requires separate reconciliation');
  audit.at=new Date().toISOString();audit.journalBeforeHash=hash(journalBefore);
  const expected=JSON.parse(journalBefore),b=expected.batches[BATCH];
  b.status='ABORTED_UNWRITTEN';b.resolution=audit;expected.active=null;
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
  }catch(e){process.stdout.write('NPG-TP1 '+JSON.stringify({ok:false,error:e.message})+'\n');process.exit(1);}
  let closed=false;function close(){if(!closed){closed=true;broker.close();}}
  process.on('exit',close);for(const signal of ['SIGINT','SIGTERM','SIGHUP'])process.on(signal,()=>process.exit(1));
  const rl=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
  process.stdout.write('NPG-TP1 {"ready":true}\n');
  rl.once('line',line=>{
    try{
      const result=apply(broker,JSON.parse(line),!process.argv.includes('--apply'));
      process.stdout.write('NPG-TP1 '+JSON.stringify({ok:true,...result})+'\n',()=>process.exit(0));
    }catch(e){process.stdout.write('NPG-TP1 '+JSON.stringify({ok:false,error:e.message})+'\n',()=>process.exit(1));}
  });
  rl.on('close',close);
}
module.exports={inspect,apply};
