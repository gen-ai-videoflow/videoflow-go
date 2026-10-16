/** Same-origin proxy with bounded admission, input size, lifetime and one forwarding hop. */
let active=0;
export async function proxyRequest(request,target,options={}){
 if(active>=(options.maxConcurrent??16))return Response.json({message:'The service is busy. Please try again shortly.'},{status:503,headers:{'Retry-After':'2'}});
 active++;
 const controller=new AbortController(),signal=AbortSignal.any([request.signal,controller.signal]);
 const timer=setTimeout(()=>controller.abort(),options.timeoutMs??30000);
 try{return await forwardRequest(request,target,{...options,signal});}
 catch{return Response.json({code:'api_unavailable',message:'The studio is temporarily unavailable. Please try again.'},{status:signal.aborted?408:502});}
 finally{clearTimeout(timer);active--;}
}
async function forwardRequest(request,target,{maxBytes=5*1024*1024,fetch=globalThis.fetch,signal}={}){
 if(request.headers.has('x-videoflow-gateway-hop')||new URL(target).origin===new URL(request.url).origin)return Response.json({message:'The API gateway connection is misconfigured.'},{status:508});
 const headers=new Headers(request.headers);
 for(const header of ['host','connection','content-length','transfer-encoding'])headers.delete(header);
 headers.set('x-videoflow-gateway-hop','1');
 let body;
 if(!['GET','HEAD'].includes(request.method)){
  if(Number(request.headers.get('content-length'))>maxBytes)return Response.json({message:'Request body is too large.'},{status:413});
  const chunks=[];let size=0;const reader=request.body?.getReader();
  if(reader){
   const cancel=()=>{void reader.cancel().catch(()=>{});};
   signal.addEventListener('abort',cancel,{once:true});
   try{
    if(signal.aborted)throw new Error('Aborted');
    while(true){const {value,done}=await reader.read();if(signal.aborted)throw new Error('Aborted');if(done)break;size+=value.byteLength;if(size>maxBytes){void reader.cancel().catch(()=>{});return Response.json({message:'Request body is too large.'},{status:413});}chunks.push(value);}
   }finally{signal.removeEventListener('abort',cancel);reader.releaseLock();}
  }
  body=new Uint8Array(size);let offset=0;for(const chunk of chunks){body.set(chunk,offset);offset+=chunk.byteLength;}
 }
 const response=await fetch(target,{method:request.method,headers,body,redirect:'manual',signal});
 const responseHeaders=new Headers(response.headers);
 for(const header of ['connection','transfer-encoding','content-encoding','content-length'])responseHeaders.delete(header);
 responseHeaders.set('cache-control','no-store');
 // Cancellation continues through response streaming after the header deadline is cleared.
 return new Response(response.body?.pipeThrough(new TransformStream(),{signal:request.signal})??null,{status:response.status,headers:responseHeaders});
}
