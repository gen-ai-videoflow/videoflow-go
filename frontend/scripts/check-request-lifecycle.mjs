import assert from 'node:assert/strict';
import { createPoller } from '../src/lib/cloud/poll.mjs';
import { fetchJSON } from '../src/lib/cloud/request.mjs';
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function until(check){const end=Date.now()+2000;while(!check()){if(Date.now()>end)throw new Error('Timed out waiting for poll lifecycle');await sleep(5);}}
let running=0,max=0,calls=0,visible=true;
const poll=createPoller(async()=>{running++;max=Math.max(max,running);calls++;await sleep(20);running--;},{interval:5,maxInterval:30,enabled:()=>visible,jitter:()=>0});
poll.start(); await until(()=>calls>=3); assert.equal(max,1,'slow poll must never overlap');
visible=false;await until(()=>running===0);const paused=calls;await sleep(25);assert.equal(calls,paused,'hidden poll must pause');
poll.stop();visible=true;await sleep(25);assert.equal(calls,paused,'stopped poll must stay stopped');
let aborted=false;
const fakeFetch=(_url,{signal})=>new Promise((_resolve,reject)=>{signal.addEventListener('abort',()=>{aborted=true;reject(signal.reason);},{once:true});});
await assert.rejects(fetchJSON('/slow',{}, {timeoutMs:10,fetch:fakeFetch}),/long|abort|timeout/i);assert.equal(aborted,true);
let attempts=0;
await assert.rejects(fetchJSON('/paid',{method:'POST'}, {fetch:async()=>{attempts++;return new Response('{"message":"Busy"}',{status:503,headers:{'Retry-After':'3'}});}}),e=>e.status===503&&e.retryAfter===3);
assert.equal(attempts,1,'mutations must not auto retry');
const controller=new AbortController();controller.abort();await assert.rejects(fetchJSON('/cancel',{}, {signal:controller.signal,fetch:fakeFetch}));
console.log('Request lifecycle: non-overlap, pause, stop, deadline, cancellation and no mutation retries passed');
