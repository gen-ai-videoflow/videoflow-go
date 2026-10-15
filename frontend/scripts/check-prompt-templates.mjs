import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {renderPrompt,variableNames,promptBytes} from '../src/lib/cloud/prompts/render.mjs';
const rows=JSON.parse(readFileSync(new URL('../src/lib/cloud/prompts/catalogue.json',import.meta.url)));
for(const row of rows){assert(row.body_en&&row.body_zh);assert.deepEqual(variableNames(row.body_en).sort(),variableNames(row.body_zh).sort(),row.id);for(const text of [row.body_en,row.body_zh]){const values=Object.fromEntries(variableNames(text).map(k=>[k,'测试 $& <script>']));const result=renderPrompt(text,values);assert.equal(result.missing.length,0);assert.equal(result.invalid,false);assert(result.text.includes('测试 $& <script>'));}}
assert.deepEqual(renderPrompt('A {{subject}} in {{setting}}',{subject:'角色'}).missing,['setting']);assert(renderPrompt('{{ref 1}}',{}).invalid);assert.equal(renderPrompt('{{subject}}',{subject:'$&'}).text,'$&');
console.log('Bilingual template parity, literal rendering, missing variables and unsupported reference guards passed.');

assert.equal(promptBytes('角色'),6);assert.equal(renderPrompt('{{subject}}',{subject:'角色'}).bytes,6);

assert.deepEqual(renderPrompt('{{constructor}}',{}).missing,['constructor']);assert.equal(renderPrompt('{{constructor}}',{constructor:'literal'}).text,'literal');
