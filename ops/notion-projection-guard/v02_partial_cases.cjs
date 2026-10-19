'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),vm=require('node:vm');
const guard=require('./broker.cjs'),{Broker,hash,pageData,atomic}=guard;
const BATCH='CENTRAL-20261019T153711Z-NK139-SORI43-GATES';
const IDS=['3f086e6dd1d1818e9aecdc093a08cda9','3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7',
 '3f586e6dd1d181ddb9f3e7217f64bfcd','3b286e6dd1d1817aa093fca807659f07','3b286e6dd1d181f2a75cfbd3885071c4',
 '3bf86e6dd1d18110938bc358e17c779e','3ed86e6dd1d18118b0b8d98bb91892fb','3b286e6dd1d1811d9308d9a2b56c9de4'];
const heads=IDS.map((_,i)=>i===1?'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3':
 i===2?'## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED':'## CURRENT CENTRAL OBSERVATION · test'+i);
const raw=(i,literal,revision='fresh')=>({structuredContent:{id:IDS[i],text:'<properties>\n{"own":"preserved"}\n</properties>\n<content>\n'+heads[i]+'\n'+literal+'\n## WORKER HISTORY\nimmutable own proof\n</content>',page_last_edited_at:revision}});
function helper(f,override={}){
 let source=fs.readFileSync(path.join(__dirname,'reconcile_v02_partial.cjs'),'utf8');
 for(const [k,v] of Object.entries({ROOT:f.root,RUNTIME:f.runtime,PLAN:hash(JSON.stringify(f.edits)),TICKET:f.ticket}))
  source=source.replace(new RegExp("const "+k+"='[^']*';"),'const '+k+'='+JSON.stringify(v)+';');
 source=source.replace(/const ESCAPE_AT=\d+;/,'const ESCAPE_AT='+f.escapeAt+';').replace(/const FILES=.+;/,'const FILES='+JSON.stringify(f.files)+';');
 for(const [n,h] of Object.entries({'broker.cjs':'b0b07630eebb9d2ebbc8a4c85bd9cdded9ca2c5432a66a8cebd45e7d8f1f102c',
 'runner.js':'278b3e544f9ba10538f73901c705262d9a3ef76232a931d67da6ed80750c6d52','policy.json':'804709b778ea5fe82eac705654d06f52af8391aa0504d8e0582f3a87022fdf1b'}))
  source=source.replaceAll(h,hash(fs.readFileSync(path.join(f.runtime,n),'utf8')));
 const module={exports:{}};vm.runInNewContext(source,{module,require:n=>n==='./broker.cjs'?{...guard,...override}:require(n),__dirname:f.runtime,process});return module.exports;
}
function fixture(t){
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'npg-v02-')),root=path.join(dir,'state'),runtime=path.join(dir,'runtime');fs.mkdirSync(runtime,{mode:0o700});
 const policy={productionEnabled:true,productionPages:IDS.filter((_,i)=>i!==1&&i!==2),canaryPages:[],headingPrefixes:['## CURRENT CENTRAL OBSERVATION'],
  centralSections:[1,2].map(i=>({pageId:IDS[i],heading:heads[i]}))};
 for(const n of ['broker.cjs','runner.js'])fs.copyFileSync(path.join(__dirname,n),path.join(runtime,n));fs.writeFileSync(path.join(runtime,'policy.json'),JSON.stringify(policy));
 const b=new Broker(root,policy);t.after(()=>{b.close();fs.rmSync(dir,{recursive:true,force:true});});
 const edits=IDS.map((pageId,i)=>({pageId,heading:heads[i],oldStr:'status=`OLD'+i+'`;',newStr:'status=`NEW'+i+'`;'}));
 edits[3].oldStr='observation=`OLD`; V02=`Lookup/GetJob reused;';edits[3].newStr='observation=`NEW`;';
 const pages=[];let escapeAt,ticket;const files={};
 for(let i=0;i<9;i++){
  if(i>=4){pages.push(raw(i,edits[i].oldStr));continue;}
  const e=edits[i],before=raw(i,e.oldStr+(i===3?' FOUND-only/N4 meanings kept`; product=`partial; merges are stage completion only`.':''));
  const page=pageData(before,IDS[i]),index=page.body.indexOf(e.oldStr),pre=page.body.slice(0,index),post=page.body.slice(index+e.oldStr.length),expected=pre+e.newStr+post;
  const snap={version:1,batchId:BATCH,index:i,ticket:'ticket-'+i,edit:e,page,expected,beforeHash:hash(pre),afterHash:hash(post),bodyHash:hash(page.body),propertiesHash:hash(page.properties),expectedHash:hash(expected)};
  atomic(b.snapshot(BATCH,i),snap);
  let content=expected;
  if(i===3){escapeAt=expected.indexOf('`.\n');content=expected.slice(0,escapeAt)+'\\'+expected.slice(escapeAt);ticket=snap.ticket;}
  const p=raw(i,'placeholder');p.structuredContent.text=p.structuredContent.text.replace(heads[i]+'\nplaceholder\n## WORKER HISTORY\nimmutable own proof',content);
  pages.push(p);const saved=pageData(p,IDS[i]);
  atomic(b.snapshot(BATCH,i)+(i===3?'.actual.json':'.verified.json'),i===3?{page:saved,actualHash:hash(saved.body),at:'original'}:{page:saved,bodyHash:hash(saved.body),at:'original'});
 }
 const receipt={ticket,snapshot:b.snapshot(BATCH,3),snapshotHash:hash(fs.readFileSync(b.snapshot(BATCH,3),'utf8'))};
 b.state={version:1,active:BATCH,batches:{[BATCH]:{planHash:hash(JSON.stringify(edits)),edits,next:3,status:'HALTED',error:'READBACK_MISMATCH',receipt}}};b.save();
 for(const n of fs.readdirSync(root))if(n.startsWith(hash(BATCH)+'-'))files[n.slice(hash(BATCH).length+1)]=hash(fs.readFileSync(path.join(root,n),'utf8'));
 const f={dir,root,runtime,policy,b,edits,pages,ticket,files,escapeAt};f.helper=helper(f);return f;
}
function unchanged(f,fn){const before=fs.readFileSync(f.b.file,'utf8');assert.throws(fn);assert.equal(fs.readFileSync(f.b.file,'utf8'),before);}
test('exact V02 closure is partial, preserves all original evidence and never acknowledges or advances a write',t=>{
 const f=fixture(t),before=JSON.parse(fs.readFileSync(f.b.file));assert.equal(f.helper.apply(f.b,f.pages).checkOnly,true);
 assert.deepEqual(JSON.parse(fs.readFileSync(f.b.file)),before);
 const r=f.helper.apply(f.b,f.pages,false),after=JSON.parse(fs.readFileSync(f.b.file));assert.equal(after.active,null);assert.equal(after.batches[BATCH].status,'RECONCILED_PARTIAL');
 for(const k of ['edits','planHash','next','receipt','error'])assert.deepEqual(after.batches[BATCH][k],before.batches[BATCH][k]);
 for(const k of ['notionWrites','acknowledgedWrites','pageAdvances'])assert.equal(r.audit[k],0);
 assert.equal(r.audit.priorVerified,3);assert.equal(r.audit.observedMisrenderedWrites,1);assert.equal(r.audit.unattemptedEdits,5);
 for(const [n,h] of Object.entries(f.files))assert.equal(hash(fs.readFileSync(path.join(f.root,hash(BATCH)+'-'+n),'utf8')),h);
 unchanged(f,()=>f.helper.apply(f.b,f.pages,false));
});
for(const [name,change] of [
 ['plan',f=>{f.b.state.batches[BATCH].edits[3].newStr='other';f.b.save();}],
 ['ticket',f=>{f.b.state.batches[BATCH].receipt.ticket='other';f.b.save();}],
 ['next',f=>{f.b.state.batches[BATCH].next=4;f.b.save();}],
 ['error',f=>{f.b.state.batches[BATCH].error='other';f.b.save();}],
 ['status',f=>{f.b.state.batches[BATCH].status='READY';f.b.save();}],
 ['other active',f=>{f.b.state.active='other';f.b.save();}],
 ['pending extra escape',f=>{f.pages[3].structuredContent.text=f.pages[3].structuredContent.text.replace('kept`','kept\\`');}],
 ['pending worker delta',f=>{f.pages[3].structuredContent.text=f.pages[3].structuredContent.text.replace('immutable','changed');}],
 ['pending revision',f=>{f.pages[3].structuredContent.page_last_edited_at='later';}],
 ['prior worker delta',f=>{f.pages[1].structuredContent.text=f.pages[1].structuredContent.text.replace('immutable','new genuine proof');}],
 ['prior properties',f=>{f.pages[0].structuredContent.text=f.pages[0].structuredContent.text.replace('preserved','new');}],
 ['changed receipt time',f=>{const p=f.b.snapshot(BATCH,1)+'.verified.json',x=JSON.parse(fs.readFileSync(p));x.at='later';atomic(p,x);}],
 ['changed actual',f=>{const p=f.b.snapshot(BATCH,3)+'.actual.json',x=JSON.parse(fs.readFileSync(p));x.at='later';atomic(p,x);}],
 ['missing receipt',f=>fs.unlinkSync(f.b.snapshot(BATCH,2)+'.verified.json')],
 ['unexpected verified pending',f=>atomic(f.b.snapshot(BATCH,3)+'.verified.json',{})],
 ['prepared fifth page',f=>atomic(f.b.snapshot(BATCH,4),{})],
 ['written remaining',f=>{f.pages[6]=raw(6,f.edits[6].newStr);}],
 ['revoked scope',f=>{f.policy.centralSections=[];}],
 ['production hold',f=>{f.policy.productionEnabled=false;}],
 ['missing read',f=>{f.pages.pop();}],
 ['wrong read order',f=>{[f.pages[0],f.pages[1]]=[f.pages[1],f.pages[0]];}],
 ['incomplete read',f=>{f.pages[5].structuredContent.unknown_block_count=1;}],
 ['changed runtime',f=>fs.appendFileSync(path.join(f.runtime,'runner.js'),'changed')],
 ['concurrent journal',f=>{const j=JSON.parse(fs.readFileSync(f.b.file));j.extra=true;atomic(f.b.file,j);}]
])test('V02 closure rejects '+name+' without clearing HALTED',t=>{const f=fixture(t);change(f);unchanged(f,()=>f.helper.apply(f.b,f.pages,false));});
test('V02 audit persistence failure retains the barrier',t=>{const f=fixture(t),h=helper(f,{atomic(){throw new Error('disk full');}});unchanged(f,()=>h.apply(f.b,f.pages,false));});
