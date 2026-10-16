/** Completion-scheduled polling: one request at a time, bounded failure backoff. */
export function createPoller(task, { interval=10000, maxInterval=60000, enabled=()=>true, jitter=Math.random, onError=()=>{} }={}) {
  let timer, stopped=true, inFlight=false, failures=0, controller;
  function schedule(){if(stopped)return;const delay=Math.min(maxInterval,interval*2**Math.min(failures,6));timer=setTimeout(tick,delay*(1+.15*jitter()));}
  async function tick(){
    if(stopped||inFlight)return;
    if(!enabled()){schedule();return;}
    inFlight=true;controller=new AbortController();
    try{await task(controller.signal);failures=0;}catch(error){if(!stopped){failures++;onError(error);}}
    finally{inFlight=false;controller=undefined;schedule();}
  }
  return {start(){if(!stopped)return;stopped=false;schedule();},stop(){stopped=true;clearTimeout(timer);controller?.abort();}, async refresh(){if(stopped||inFlight)return;clearTimeout(timer);await tick();}};
}
