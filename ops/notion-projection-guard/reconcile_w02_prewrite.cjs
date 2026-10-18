#!/usr/bin/env node
'use strict';
// One reviewed pre-write incident only. This is not a broker action or a
// generic HALTED reset. It emits no Notion request and acknowledges no write.
const fs=require('node:fs'),path=require('node:path'),readline=require('node:readline');
const {Broker,hash,pageData,plan,atomic}=require('./broker.cjs');
const ROOT='/home/heain/.local/state/notion-projection-guard';
const POLICY='/home/heain/.local/share/notion-projection-guard/policy.json';
const BATCH='SORI-W02-GUARD-PROJECTIONS-20261019';
const PLAN='5b04d7edfa397f94408844b39b89a6f89998f1b5b7483dc7cafdd7aa3ff17f95';
const ERROR='PROPERTY_READBACK_MISMATCH 3f586e6dd1d181ebb43dd7de62aa2ca4';
const SOURCE='SORI-W02-GUARD-FOLLOWTHROUGH-20261019';
const SOURCE_PLAN='25e697835128ec1d84188eff8230c17274f4ba2640cb5f974339ceaea35c146f';
const WRITER_PROPS='71604a9b89ddb23a362ad5ad96be29b7600f011ffc8c08ece96a096f4ab2de92';
const IDS=['3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7'];
function must(ok,message){if(!ok)throw new Error('W02_RECONCILE: '+message);}
function read(file){const st=fs.lstatSync(file);must(st.isFile()&&!st.isSymbolicLink()&&!(st.mode&0o077),'Private regular evidence required');return JSON.parse(fs.readFileSync(file,'utf8'));}
function inspect(broker,raw){
  const b=broker.state.batches[BATCH],source=broker.state.batches[SOURCE];
  must(broker.state.active===BATCH&&b?.status==='HALTED'&&b.next===0&&b.receipt===null&&
    b.error===ERROR&&b.edits.length===4&&b.planHash===PLAN&&hash(JSON.stringify(b.edits))===PLAN,'Original zero-progress incident required');
  must(!fs.readdirSync(broker.root).some(name=>name.startsWith(hash(BATCH)+'-')),'No page may have been prepared or authorized');
  must(source?.status==='COMPLETED'&&source.next===2&&source.receipt===null&&
    source.planHash===SOURCE_PLAN&&hash(JSON.stringify(source.edits))===SOURCE_PLAN,'Original verified source batch required');
  must(Array.isArray(raw)&&raw.length===2,'Two fresh complete source reads required');
  const receipts=[];
  for(let i=0;i<2;i++){
    const edit=source.edits[i],file=broker.snapshot(SOURCE,i),snap=read(file),verified=read(file+'.verified.json');
    must(edit.pageId===IDS[i]&&snap.batchId===SOURCE&&snap.index===i&&
      JSON.stringify(snap.edit)===JSON.stringify(edit)&&snap.ticket&&
      hash(snap.page.body)===snap.bodyHash&&hash(snap.page.properties)===snap.propertiesHash&&
      hash(snap.expected)===snap.expectedHash&&plan(snap.page,edit).expected===snap.expected,'Original snapshot changed');
    const preimage=pageData(snap.page.raw,IDS[i]),saved=pageData(verified.page.raw,IDS[i]);
    must(preimage.body===snap.page.body&&preimage.properties===snap.page.properties&&preimage.revision===snap.page.revision&&
      saved.body===snap.expected&&saved.body===verified.page.body&&verified.bodyHash===snap.expectedHash&&
      saved.properties===snap.page.properties&&saved.properties===verified.page.properties&&
      saved.revision===verified.page.revision&&!verified.reconciliation,'Original protected readback changed');
    const fresh=pageData(raw[i],IDS[i]);must(fresh.revision&&fresh.body===saved.body,'Fresh worker/body change');
    if(i===0){
      // Key ordering is locale-independent here; values remain exact.
      const canonical=JSON.stringify(Object.fromEntries(Object.entries(JSON.parse(fresh.properties)).sort(([a],[b])=>a<b?-1:a>b?1:0)));
      must(hash(canonical)===WRITER_PROPS,'Unexpected native property change');
      const before=JSON.parse(saved.properties),after=JSON.parse(fresh.properties);
      const allowed=new Set(['내용 요약','작업 상태','다음 담당자','큐 구획','재개 지시','차단 대상','저장소 식별자','큐 정리 근거','date:큐 정리 시각:start','date:큐 정리 시각:is_datetime']);
      for(const key of new Set([...Object.keys(before),...Object.keys(after)]))
        if(!allowed.has(key))must(JSON.stringify(before[key])===JSON.stringify(after[key]),'Protected worker property changed');
      must(after['date:큐 정리 시각:start']==='2026-10-18T21:54:00.000Z'&&after['date:큐 정리 시각:is_datetime']===1,'Only observed minute precision admitted');
    }else must(fresh.properties===saved.properties&&fresh.revision===saved.revision,'Paired source changed');
    receipts.push({pageId:IDS[i],bodyHash:hash(fresh.body),propertiesHash:hash(fresh.properties),revision:fresh.revision,
      originalSnapshotHash:hash(JSON.stringify(snap)),verifiedHash:hash(JSON.stringify(verified))});
  }
  return {kind:'REVIEWED_W02_PREWRITE_METADATA_PRECISION',batchId:BATCH,planHash:PLAN,error:ERROR,
    next:0,ticket:null,notionBodyWrites:0,acknowledgedWrites:0,metadataWritesObserved:1,
    requestedDate:'2026-10-18T21:54:01.538Z',storedDate:'2026-10-18T21:54:00.000Z',receipts};
}
if(require.main===module){
  let broker;
  try{broker=new Broker(ROOT,JSON.parse(fs.readFileSync(POLICY,'utf8')));}
  catch(e){process.stdout.write('W02-R1 '+JSON.stringify({ok:false,error:e.message})+'\n');process.exit(1);}
  const rl=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
  process.stdout.write('W02-R1 {"ready":true}\n');
  let closed=false;function close(){if(!closed){closed=true;broker.close();}}
  process.on('exit',close);for(const signal of ['SIGINT','SIGTERM','SIGHUP'])process.on(signal,()=>process.exit(1));
  rl.once('line',line=>{
    try{
      const raw=JSON.parse(line),audit=inspect(broker,raw);
      if(!process.argv.includes('--check-only')){
        audit.at=new Date().toISOString();
        atomic(path.join(ROOT,hash(BATCH)+'.w02-metadata-reconciliation.json'),{audit,freshPages:raw});
        const b=broker.state.batches[BATCH];
        b.reconciliations=[...(b.reconciliations??[]),audit];b.status='READY';broker.save();
      }
      process.stdout.write('W02-R1 '+JSON.stringify({ok:true,checkOnly:process.argv.includes('--check-only'),audit})+'\n',()=>process.exit(0));
    }catch(e){process.stdout.write('W02-R1 '+JSON.stringify({ok:false,error:e.message})+'\n',()=>process.exit(1));}
  });
  rl.on('close',close);
}
module.exports={inspect};
