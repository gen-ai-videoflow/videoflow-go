export class CloudError extends Error {
  constructor(message,status,retryAfter=0){super(message);this.name='CloudError';this.status=status;this.retryAfter=retryAfter;}
}
/** No implicit retries: callers own mutation idempotency and retry policy. */
export async function fetchJSON(url,init={},options={}){
  const controller=new AbortController();
  const cancel=()=>controller.abort(options.signal?.reason);
  if(options.signal?.aborted)cancel();else options.signal?.addEventListener('abort',cancel,{once:true});
  const timer=setTimeout(()=>controller.abort(new Error('The request took too long. Please try again.')),options.timeoutMs??30000);
  try{
    if(controller.signal.aborted)throw controller.signal.reason;
    const response=await (options.fetch??globalThis.fetch)(url,{...init,signal:controller.signal});
    const data=await response.json().catch(()=>({}));
    if(!response.ok){const raw=response.headers.get('Retry-After');const seconds=raw?Number(raw):0;throw new CloudError(data.message||`Request failed (${response.status}). Please try again.`,response.status,Number.isFinite(seconds)?Math.max(0,seconds):0);}
    return data;
  }finally{clearTimeout(timer);options.signal?.removeEventListener('abort',cancel);}
}
