(function createCompleteProjection(tools, config) {
  'use strict';
  // Code Mode entry point. It adds a mandatory projection manifest while all
  // body mutations still use the separately approved durable broker/runner.
  const shared=[
    ['3b286e6dd1d1817aa093fca807659f07','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · operator_queue'],
    ['3b286e6dd1d181f2a75cfbd3885071c4','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · guardrail_queue'],
    ['3bf86e6dd1d18110938bc358e17c779e','## CURRENT INCIDENT MAINTENANCE · BODY_RANGE_OVERWRITE-20261011T2334Z · RESTORED_VERIFIED / ADOPTION_PENDING'],
    ['3ed86e6dd1d18118b0b8d98bb91892fb','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · hub'],
    ['3b286e6dd1d1811d9308d9a2b56c9de4','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · dashboard']];
  const repos={
    nodekit:{native:['3f386e6dd1d18110bf66c5204284ed46','3f386e6dd1d181b5b467e20fafa69365'],
      bodies:[['3f086e6dd1d1818e9aecdc093a08cda9','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · nodekit_index']]},
    sori:{native:['3f586e6dd1d181ebb43dd7de62aa2ca4','3f586e6dd1d181cd9f5bdcdab6290de7'],
      bodies:[['3f586e6dd1d181ebb43dd7de62aa2ca4','## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A KEEP_GATED ON W01.3'],
        ['3f586e6dd1d181cd9f5bdcdab6290de7','## CURRENT · 마지막 중앙 관측 · QSYNC-20261010T145034Z-v6 · W02.A REVIEW KEEP_GATED'],
        ['3f586e6dd1d181ddb9f3e7217f64bfcd','## CURRENT CENTRAL OBSERVATION · NPR-20261012 · sori_index']]}};
  const requiredProps=['작업 상태','다음 담당자','큐 구획','차단 대상','재개 지시','큐 정리 근거'];
  const centralProps=new Set([...requiredProps,'설계에이전트 답변','저장소 식별자',
    'date:큐 정리 시각:start','date:큐 정리 시각:is_datetime']);
  const must=(ok,why)=>{if(!ok)throw new Error('PROJECTION_BUNDLE: '+why);};
  const stable=value=>JSON.stringify(Object.fromEntries(Object.entries(value).sort(([a],[b])=>a.localeCompare(b))));
  const count=(text,literal)=>text.split(literal).length-1;
  function data(raw,id){
    for(let n=0;n<5&&raw?.structuredContent;n++){
      must(!raw.isError,'Fetch failed');raw=raw.structuredContent;
    }
    must(raw&&!raw.isError&&!raw.error&&raw.id?.replaceAll('-','').toLowerCase()===id,'Wrong or failed page');
    const text=raw.text;
    must(raw.truncated!==true&&!(raw.unknown_block_count??0)&&!(raw.unknown_block_ids?.length??0)&&
      typeof text==='string'&&!/<unknown\b|content (?:has been |is )?truncated/i.test(text)&&
      count(text,'<content>\n')===1&&count(text,'\n</content>')===1,'Complete native page required');
    const start=text.indexOf('<content>\n')+10,stop=text.indexOf('\n</content>',start);
    const props=text.match(/<properties>\n([\s\S]*?)\n<\/properties>/);
    must(stop>=start&&props&&typeof raw.page_last_edited_at==='string','Complete properties and revision required');
    return {body:text.slice(start,stop),props:JSON.parse(props[1]),revision:raw.page_last_edited_at};
  }
  function manifest(bundle){
    must(bundle&&/^[A-Za-z0-9_-]{1,100}$/.test(bundle.observationId),'Literal observation ID required');
    must(Array.isArray(bundle.repositories)&&bundle.repositories.length>0&&
      new Set(bundle.repositories).size===bundle.repositories.length&&bundle.repositories.every(r=>Object.hasOwn(repos,r)),'Known distinct repositories required');
    const bodies=bundle.repositories.flatMap(r=>repos[r].bodies).concat(shared);
    const native=bundle.repositories.flatMap(r=>repos[r].native);
    must(Array.isArray(bundle.edits)&&bundle.edits.length===bodies.length&&
      new Set(bundle.edits.map(e=>e.pageId)).size===bodies.length,'Every mandatory body target required exactly once');
    must(Array.isArray(bundle.native)&&bundle.native.length===native.length&&
      new Set(bundle.native.map(n=>n.pageId)).size===native.length,'Every native metadata target required exactly once');
    const marker='observation_id=`'+bundle.observationId+'`';
    const edits=bodies.map(([id,heading])=>{
      const e=bundle.edits.find(e=>e.pageId===id);
      must(e&&e.heading===heading&&typeof e.oldStr==='string'&&e.oldStr.length>0&&e.oldStr!==e.newStr&&
        typeof e.newStr==='string'&&e.newStr.includes(marker),'Wrong target/heading or missing shared observation ID');
      return e;
    });
    for(const id of native){
      const n=bundle.native.find(n=>n.pageId===id),p=n?.properties;
      must(p&&requiredProps.every(k=>Object.hasOwn(p,k))&&Object.keys(p).every(k=>centralProps.has(k))&&
        typeof p['큐 정리 근거']==='string'&&p['큐 정리 근거'].includes(marker),'Native central metadata manifest required');
    }
    return {edits,native,ids:[...new Set([...native,...bodies.map(([id])=>id)])]};
  }
  async function readAll(ids){
    const pages=new Map();
    // Bounded reads avoid flooding the connector with all complete page bodies.
    for(let i=0;i<ids.length;i+=3){
      const group=await Promise.all(ids.slice(i,i+3).map(async id=>[id,data(await tools.mcp__codex_apps__notion_fetch({id}),id)]));
      for(const [id,p] of group)pages.set(id,p);
    }
    return pages;
  }
  function metadata(bundle,pages){
    for(const n of bundle.native)for(const [k,v] of Object.entries(n.properties))
      must(stable({v:pages.get(n.pageId).props[k]})===stable({v}),'Native property differs: '+n.pageId+'/'+k);
  }
  async function run(bundle){
    // Deep copy before awaits: mutable caller input must not change the plan.
    bundle=JSON.parse(JSON.stringify(bundle));
    const m=manifest(bundle),before=await readAll(m.ids);
    metadata(bundle,before); // Properties are reconciled separately, with their own preservation readback.
    for(const e of m.edits){
      const b=before.get(e.pageId).body;
      must(count(b,e.oldStr)===1||count(b,e.newStr)===1,'Planned literal missing or ambiguous');
    }
    const runner=config.createRunner(tools,config.runnerConfig);
    const receipt=await runner.run(bundle.observationId,m.edits);
    must(receipt.status==='COMPLETED','Body batch incomplete');
    const after=await readAll(m.ids);metadata(bundle,after);
    for(const id of m.ids){
      const a=before.get(id),b=after.get(id),e=m.edits.find(e=>e.pageId===id);
      const expected=e&&count(a.body,e.newStr)!==1?a.body.replace(e.oldStr,e.newStr):a.body;
      must(b.body===expected&&stable(b.props)===stable(a.props),'Source/protected content changed: '+id);
      if(e)must(count(b.body,e.newStr)===1,'Final target absent: '+id);
    }
    return {status:'COMPLETE',observationId:bundle.observationId,repositories:bundle.repositories,
      bodyTargets:m.edits.length,nativeTargets:m.native.length,protectedContent:'UNCHANGED',
      receipt,readback:m.ids.map(id=>({pageId:id,revision:after.get(id).revision}))};
  }
  return {run};
})
