(function createProjectionRunner(tools, config) {
  'use strict';
  // Evaluated as a function expression in Code Mode; no Node imports are used.
  const deviceId = config.deviceId;
  const command = config.command;
  let pid, sequence = 0, closed = false;
  function textOf(result) { return (result?.content ?? []).filter(x => x.type === 'text').map(x => x.text).join('\n'); }
  async function settled(value) {
    for (let n = 0; n < 12; n++) {
      const callId = value.call_id ?? value.structuredContent?.call_id ?? textOf(value).match(/call_id[": =]+([a-zA-Z0-9_-]+)/)?.[1];
      if (!callId) return value;
      value = await tools.mcp__codex_apps__remote_desktop_commander_get_tool_result({ call_id: callId });
      if (value.isError) throw new Error('REMOTE_CALL_FAILED: '+textOf(value).slice(0,300));
      if (!/pending|executing|running/i.test(value.structuredContent?.status ?? '') && textOf(value).includes('NPG/1 ')) return value;
    }
    throw new Error('REMOTE_RESULT_UNKNOWN');
  }
  function response(result, requestId) {
    if (result.isError) throw new Error('REMOTE_ERROR: '+textOf(result).slice(0,300));
    const values = textOf(result).split('\n').filter(line => line.startsWith('NPG/1 ')).map(line => JSON.parse(line.slice(6)));
    return values.find(value => value.requestId === requestId);
  }
  async function start() {
    if (closed) throw new Error('BROKER_CLOSED: create a new runner for a new batch');
    if (pid) return;
    const result = await settled(await tools.mcp__codex_apps__remote_desktop_commander_start_process({ deviceId, command, timeout_ms: 3000 }));
    pid = Number(textOf(result).match(/PID (\d+)/)?.[1]);
    if (!pid) throw new Error('BROKER_NOT_READY: '+textOf(result).slice(0,500));
    let ready=textOf(result).includes('"ready":true');
    for(let n=0;!ready && n<12;n++) {
      const output=await settled(await tools.mcp__codex_apps__remote_desktop_commander_read_process_output({deviceId,pid,timeout_ms:3000,length:30}));
      if(textOf(output).includes('"code":'))throw new Error('BROKER_NOT_READY: '+textOf(output).slice(0,500));
      ready=textOf(output).includes('"ready":true');
    }
    if(!ready)throw new Error('BROKER_NOT_READY');
  }
  async function broker(request) {
    await start();
    const requestId = 'projection-'+pid+'-'+(++sequence);
    const result = await settled(await tools.mcp__codex_apps__remote_desktop_commander_interact_with_process({
      deviceId, pid, input: JSON.stringify({ ...request, requestId })+'\n', timeout_ms: 3000, wait_for_prompt: false }));
    let value = response(result, requestId);
    for (let n=0; !value && n<12; n++) {
      value = response(await settled(await tools.mcp__codex_apps__remote_desktop_commander_read_process_output({ deviceId, pid, timeout_ms: 3000, length: 30 })), requestId);
    }
    if (!value) throw new Error('BROKER_RESPONSE_UNKNOWN');
    if (!value.ok) { const error = new Error(value.code+': '+value.error); error.code=value.code; throw error; }
    return value.result;
  }
  function asyncTask(result) {
    const data = result.structuredContent;
    return data?.async_task ?? data?.result?.async_task;
  }
  async function update(request) {
    let result = await tools.mcp__codex_apps__notion_notion_update_page(request);
    if (result.isError || result.structuredContent?.error) throw new Error('NOTION_UPDATE_FAILED');
    const task = asyncTask(result);
    if (task) {
      if (!task.task_id) throw new Error('ASYNC_RESULT_UNKNOWN');
      let completed=false;
      for (let n=0; n<20; n++) {
        await new Promise(resolve => setTimeout(resolve,1000));
        result = await tools.mcp__codex_apps__notion_notion_get_async_task({ task_id:task.task_id });
        if (result.isError) throw new Error('ASYNC_FAILED');
        let data=result;
        for (let j=0;j<5 && data.structuredContent;j++) data=data.structuredContent;
        if (data.status === 'failed') throw new Error('ASYNC_FAILED');
        if (data.status === 'succeeded') { completed=true; break; }
        if (!['queued','running','retrying'].includes(data.status)) throw new Error('ASYNC_RESULT_UNKNOWN');
      }
      if (!completed) throw new Error('ASYNC_PENDING_RECONCILE_REQUIRED');
    } else if (/"?async_task"?\s*[:=]/.test(textOf(result))) throw new Error('ASYNC_RESULT_UNKNOWN');
    if (!result.structuredContent || !result.content?.length) throw new Error('NOTION_RESULT_UNKNOWN');
    return result;
  }
  async function close() {
    if (pid && !closed) {
      // Graceful broker close releases only the process lock. Durable pending/HALTED
      // state survives; no journal reset or retry is performed by this adapter.
      await broker({ action:'close' });
      closed = true;
    }
  }
  async function run(batchId, edits) {
    const receipts = [];
    try {
      const begin = await broker({ action: 'begin', batchId, edits });
      if (begin.status === 'COMPLETED') return { status: 'COMPLETED', replayed: false, alreadyConsumed: true, receipts };
      if (begin.status !== 'READY') throw new Error('UNRESOLVED_BATCH: '+begin.status);
      for (let index=begin.next; index<edits.length; index++) {
        const edit=edits[index];
        const prepared = await broker({ action:'prepare', batchId, page:await tools.mcp__codex_apps__notion_fetch({ id:edit.pageId }) });
        const authorized = await broker({ action:'authorize', batchId, ticket:prepared.ticket,
          page:await tools.mcp__codex_apps__notion_fetch({ id:edit.pageId }) });
        // The caller never constructs a Notion mutation. Only the broker emits it.
        await update(authorized.request);
        const verified = await broker({ action:'verify', batchId, ticket:prepared.ticket,
          page:await tools.mcp__codex_apps__notion_fetch({ id:edit.pageId }) });
        receipts.push({ pageId:edit.pageId, ...verified, snapshot:prepared.snapshot });
      }
      return { status:'COMPLETED', receipts };
    } catch (error) {
      // Errors before begin (e.g. PRODUCTION_HOLD) need no journal mutation.
      // Errors after begin keep the global barrier, including ACK loss/crashes.
      try { await broker({ action:'halt', batchId, reason:error.message }); } catch { /* Existing state remains closed. */ }
      throw error;
    } finally { await close(); }
  }
  return { run, broker, close };
})
