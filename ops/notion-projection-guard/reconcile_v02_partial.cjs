#!/usr/bin/env node
'use strict';
// One reviewed incident closure, not a successful write acknowledgement.
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {Broker,hash,pageData,plan,atomic,validateEdit}=require('./broker.cjs');
const ROOT='/home/heain/.local/state/notion-projection-guard';
const RUNTIME='/home/heain/.local/share/notion-projection-guard';
const BATCH='CENTRAL-20261019T153711Z-NK139-SORI43-GATES';
const PLAN='8b94d81f45f31e63f98c113a9fd28ecc921280cf866fb91bddbe6772053d437a';
const TICKET='37b84f05-3f41-44b5-9282-706bc1bccf40';
const ESCAPE_AT=1162;
const IDS=['3f086e6dd1d1818e9aecdc093a08cda9','3f586e6dd1d181ebb43dd7de62aa2ca4',
  '3f586e6dd1d181cd9f5bdcdab6290de7','3f586e6dd1d181ddb9f3e7217f64bfcd',
  '3b286e6dd1d1817aa093fca807659f07','3b286e6dd1d181f2a75cfbd3885071c4',
  '3bf86e6dd1d18110938bc358e17c779e','3ed86e6dd1d18118b0b8d98bb91892fb','3b286e6dd1d1811d9308d9a2b56c9de4'];
const PINS={'broker.cjs':'b0b07630eebb9d2ebbc8a4c85bd9cdded9ca2c5432a66a8cebd45e7d8f1f102c',
  'runner.js':'278b3e544f9ba10538f73901c705262d9a3ef76232a931d67da6ed80750c6d52',
  'policy.json':'804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b'};
const FILES={"0.json":"945f190a39b7ff445a0285c48086642f4248af0a15d158ea8d4686ea478e6bea","0.json.verified.json":"a8f76b0dbc618f014139852b14c3bd6292dbd8e672f63bb3c6e08cc00004892b","1.json":"65f75dafda4b121a9852eb983c7899a51d42eaa68a904f577d506e17158d5ad9","1.json.verified.json":"0b82d3d9706760f74543e5442699d0056fe41cb16d0d6cf044a68708da438069","2.json":"f275dda949d448d607d85d8f88b6368ebd339930f29e8f0769203a9946bb4339","2.json.verified.json":"da37ec57071574ca22781d2901f208fec2d08f7cf8d929a427f9817299556d71","3.json":"8741a0b684e9144b2834508b0e72c27372f38815f2ba21cdd972d6dce7f31e66","3.json.actual.json":"7af6167e8c32e59ebc302c8c1e28fa5ef25f317503e782622708d267667bb2ba"};
function must(ok,why){if(!ok)throw new Error('V02_PARTIAL: '+why);}
function read(file){const st=fs.lstatSync(file);must(st.isFile()&&!st.isSymbolicLink()&&!(st.mode&0o077),'Private regular evidence required');return JSON.parse(fs.readFileSync(file,'utf8'));}
function pins(){
  for(const [n,h] of Object.entries(PINS))must(hash(fs.readFileSync(path.join(RUNTIME,n),'utf8'))===h,'Runtime changed: '+n);
  must(hash(fs.readFileSync(path.join(__dirname,'broker.cjs'),'utf8'))===PINS['broker.cjs'],'Helper broker differs');
}
function inspect(broker,raw){
  const b=broker.state.batches[BATCH],prefix=hash(BATCH)+'-';
  must(broker.state.active===BATCH&&b?.status==='HALTED'&&b.error==='READBACK_MISMATCH'&&b.next===3&&
    b.planHash===PLAN&&hash(JSON.stringify(b.edits))===PLAN&&b.edits.length===9&&
    b.receipt?.ticket===TICKET&&b.receipt.snapshot===broker.snapshot(BATCH,3)&&
    b.receipt.snapshotHash===FILES['3.json'],'Exact original halted batch/plan/ticket required');
  must(JSON.stringify(read(broker.file))===JSON.stringify(broker.state),'Journal changed outside lock');
  must(b.edits.every((e,i)=>e.pageId===IDS[i]),'Ordered original scope changed');
  must(broker.handle({action:'begin',batchId:BATCH,edits:b.edits}).status==='HALTED','Original scope/policy revoked');
  for(const n of fs.readdirSync(broker.root))if(n.startsWith(prefix))must(Object.hasOwn(FILES,n.slice(prefix.length)),'Unexpected prepared or acknowledged evidence');
  for(const [n,h] of Object.entries(FILES))must(hash(fs.readFileSync(path.join(broker.root,prefix+n),'utf8'))===h,'Original receipt changed: '+n);
  must(Array.isArray(raw)&&raw.length===9,'Nine complete ordered fresh pages required');
  const pages=raw.map((v,i)=>pageData(v,IDS[i]));
  for(let i=0;i<4;i++){
    const file=broker.snapshot(BATCH,i),s=read(file),e=b.edits[i],p=pageData(s.page.raw,IDS[i]);
    validateEdit(e,broker.policy);
    const at=s.page.body.indexOf(e.oldStr),pre=s.page.body.slice(0,at),post=s.page.body.slice(at+e.oldStr.length);
    must(s.batchId===BATCH&&s.index===i&&typeof s.ticket==='string'&&s.ticket&&
      JSON.stringify(s.edit)===JSON.stringify(e)&&at>=0&&s.page.body.indexOf(e.oldStr,at+1)<0&&
      p.body===s.page.body&&p.properties===s.page.properties&&p.revision===s.page.revision&&
      hash(p.body)===s.bodyHash&&hash(p.properties)===s.propertiesHash&&
      s.expected===pre+e.newStr+post&&hash(s.expected)===s.expectedHash&&
      hash(pre)===s.beforeHash&&hash(post)===s.afterHash,'Complete canonical preimage/plan changed');
    let saved;
    if(i<3){
      const v=read(file+'.verified.json');saved=pageData(v.page.raw,IDS[i]);
      must(!v.reconciliation&&typeof v.at==='string'&&saved.body===s.expected&&v.bodyHash===s.expectedHash&&
        saved.body===v.page.body&&saved.properties===s.page.properties&&saved.properties===v.page.properties&&
        saved.revision===v.page.revision,'Prior full receipt changed');
    }else{
      const v=read(file+'.actual.json');saved=pageData(v.page.raw,IDS[i]);
      must(s.ticket===TICKET&&s.expected[ESCAPE_AT]==='`'&&s.expected.slice(ESCAPE_AT-32,ESCAPE_AT).endsWith('merges are stage completion only')&&
        saved.body===s.expected.slice(0,ESCAPE_AT)+'\\'+s.expected.slice(ESCAPE_AT)&&
        saved.body===v.page.body&&saved.properties===s.page.properties&&saved.properties===v.page.properties&&
        saved.revision===v.page.revision&&hash(saved.body)===v.actualHash,'Difference exceeds the one recorded central closing-backtick escape');
    }
    must(pages[i].body===saved.body&&pages[i].properties===saved.properties&&pages[i].revision===saved.revision,'Fresh applied page changed; preserve later evidence');
  }
  for(let i=4;i<9;i++)plan(pages[i],b.edits[i]); // Still unwritten; do not replay obsolete instructions.
  return {kind:'REVIEWED_V02_PARTIAL_CLOSURE',batchId:BATCH,planHash:PLAN,next:3,
    priorVerified:3,observedMisrenderedWrites:1,unattemptedEdits:5,notionWrites:0,acknowledgedWrites:0,pageAdvances:0,
    mismatch:{pageId:IDS[3],kind:'SINGLE_ESCAPE_IN_CENTRAL_SUFFIX',escapeAt:ESCAPE_AT},
    protectedContent:'PRIOR_3_VERIFIED; PENDING_ESCAPE_EXPLICITLY_UNACCEPTED',
    repairRequired:IDS[3],files:FILES,fresh:pages.map((p,i)=>({pageId:IDS[i],bodyHash:hash(p.body),propertiesHash:hash(p.properties),revision:p.revision}))};
}
function apply(broker,raw,checkOnly=true){
  pins();must(broker.root===ROOT,'Canonical state required');
  const before=fs.readFileSync(broker.file,'utf8'),audit=inspect(broker,raw);
  must(fs.readFileSync(broker.file,'utf8')===before,'Journal changed during inspection');
  if(checkOnly)return {checkOnly:true,audit};
  const auditFile=path.join(ROOT,hash(BATCH)+'.v02-partial-closure.json');must(!fs.existsSync(auditFile),'Existing audit requires reconciliation');
  audit.at=new Date().toISOString();audit.journalBeforeHash=hash(before);
  const after=JSON.parse(before);after.batches[BATCH].status='RECONCILED_PARTIAL';after.batches[BATCH].resolution=audit;after.active=null;
  atomic(auditFile,{audit,journalBefore:JSON.parse(before),freshPages:raw});
  must(fs.readFileSync(broker.file,'utf8')===before,'Journal changed before explicit closure');
  atomic(broker.file,after);must(JSON.stringify(read(broker.file))===JSON.stringify(after),'Closure readback differs');broker.state=after;
  return {checkOnly:false,status:'RECONCILED_PARTIAL',active:null,next:3,auditFile,audit};
}
if(require.main===module){
  let broker;
  try{
    must(process.argv.slice(2).every(a=>['--check-only','--apply'].includes(a))&&!(process.argv.includes('--check-only')&&process.argv.includes('--apply')),'Choose check-only or explicit apply');
    pins();broker=new Broker(ROOT,JSON.parse(fs.readFileSync(path.join(RUNTIME,'policy.json'),'utf8')));
  }catch(e){process.stdout.write('NPG-V02 '+JSON.stringify({ok:false,error:e.message})+'\n');process.exit(1);}
  let closed=false;function close(){if(!closed){closed=true;broker.close();}}
  process.on('exit',close);for(const s of ['SIGINT','SIGTERM','SIGHUP'])process.on(s,()=>process.exit(1));
  const rl=readline.createInterface({input:process.stdin,crlfDelay:Infinity});process.stdout.write('NPG-V02 {"ready":true}\n');
  rl.once('line',line=>{try{const r=apply(broker,JSON.parse(line),!process.argv.includes('--apply'));process.stdout.write('NPG-V02 '+JSON.stringify({ok:true,...r})+'\n',()=>process.exit(0));}
    catch(e){process.stdout.write('NPG-V02 '+JSON.stringify({ok:false,error:e.message})+'\n',()=>process.exit(1));}});rl.on('close',close);
}
module.exports={inspect,apply};
