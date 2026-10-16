import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';
const source=readFileSync(new URL('../src/app.html',import.meta.url),'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
function migrate(values,fail=false){const storage={...values};Object.defineProperties(storage,{getItem:{value:key=>Object.hasOwn(storage,key)?storage[key]:null},setItem:{value:(key,value)=>{if(fail&&key.startsWith('6eek.'))throw Error('storage unavailable');storage[key]=String(value);}},removeItem:{value:key=>delete storage[key]}});runInNewContext(source,{localStorage:storage,document:{documentElement:{classList:{toggle(){},add(){}}}},window:{matchMedia:()=>({matches:false})}});return storage;}
const legacy={'videoflow.theme':'light','videoflow.cloud.project.account':'project-id','videoflow.editor.account.project':'{"version":1,"timeline":[]}',unrelated:'preserved'};
const result=migrate(legacy);assert.equal(result['6eek.theme'],'light');assert.equal(result['6eek.cloud.project.account'],'project-id');assert.equal(result['6eek.editor.account.project'],legacy['videoflow.editor.account.project']);assert.equal(result.unrelated,'preserved');assert.equal(result['videoflow.editor.account.project'],undefined);
const newer=migrate({...legacy,'6eek.theme':'dark'});assert.equal(newer['6eek.theme'],'dark');
const blocked=migrate(legacy,true);for(const [key,value] of Object.entries(legacy))assert.equal(blocked[key],value,'Existing data must survive a blocked write');
console.log('Brand migration preserves saved projects, timelines, preferences and blocked-write evidence');
