import { get, post, put } from '$lib/api';
import { subscribeJobs } from '$lib/sse';
import { toast } from 'svelte-sonner';

export type Op = {
  id: string;
  kind: string;
  status: 'running' | 'succeeded' | 'failed' | 'reconciliation_required';
  scene_id?: string | null;
  output_id?: string | null;
  error?: string | null;
  result_json?: Record<string, any> | null;
};

type OpHandlers = {
  onDone?: (op: Op) => unknown;
  onFail?: (op: Op) => unknown;
  label: string;
  method?: 'POST' | 'PUT';
  signal?:AbortSignal;
};

/**
 * POST (or PUT) `path` with ?background=true (202 -> {op_id}), then resolve
 * completion via the SSE stream (filtered on op_id) plus a 5s polling fallback
 * on GET /ops/{op_id}. Toasts start/success/failure; cleans up when terminal.
 * Throws only if the initial request fails.
 */
export async function runBackgroundOp(
  path: string,
  body: unknown,
  { onDone, onFail, label, method = 'POST',signal }: OpHandlers
): Promise<void> {
  const sep = path.includes('?') ? '&' : '?';
  const send = method === 'PUT' ? put : post;
  const started = await send(`${path}${sep}background=true`, body);
  const opId: string = started.op_id;
  toast.info(`${label} started`);

  let settled=false;
  let failures=0;
  const deadline=Date.now()+15*60*1000;
  let unsubscribe:(()=>void)|null=null;
  let pollTimer:ReturnType<typeof setInterval>|null=null;
  const cleanup=()=>{unsubscribe?.();if(pollTimer)clearInterval(pollTimer);signal?.removeEventListener('abort',abort)};
  const abort=()=>{settled=true;cleanup()};
  function settle(op:Op){
    if(settled)return;settled=true;cleanup();
    const callback=op.status==='succeeded'?onDone:onFail;
    if(op.status==='succeeded')toast.success(`${label} done`);else toast.error(op.error||`${label} failed`);
    void Promise.resolve().then(()=>callback?.(op)).catch((error)=>toast.error(error.message||'Could not refresh the completed operation'));
  }
  async function check(){
    if(settled)return;
    if(Date.now()>deadline){settle({id:opId,kind:'tracking',status:'failed',error:'Tracking timed out. Check Activity and saved outputs before starting another request.'});return}
    try{const op:Op=await get(`/ops/${opId}`);failures=0;if(op.status!=='running')settle(op)}
    catch{failures++;if(failures>=12)settle({id:opId,kind:'tracking',status:'failed',error:'Cannot reach the server. Review Activity before retrying; the operation may still be running.'})}
  }
  if(signal?.aborted)return;
  signal?.addEventListener('abort',abort,{once:true});
  unsubscribe=subscribeJobs(e=>{if(e.op_id===opId&&e.status!=='running')void check()});
  pollTimer=setInterval(()=>void check(),5000);
  void check();
}
