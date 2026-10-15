import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
const source=readFileSync(new URL('../src/lib/cloud/payment-state.ts',import.meta.url),'utf8');
const code=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText;
const {canDeposit,blockedPayment,restoredInvoiceAttempt}=await import('data:text/javascript;base64,'+Buffer.from(code).toString('base64'));
let checks=0;
for(const status of ['pending','processing','review','failed','expired','settled','unknown']) {
  for(const address of ['','  ','0x123']) {
    assert.equal(canDeposit({status,address}),['pending','processing'].includes(status)&&address==='0x123');checks++;
  }
}
for(const status of ['review','failed','expired','unknown']){assert.ok(blockedPayment({status,address:''}).message.includes('support'));checks++;}
const attempt={key:'abcdefgh-1234',id:'invoice-123'};
assert.deepEqual(restoredInvoiceAttempt(JSON.parse(JSON.stringify(attempt))),attempt);checks++;
for(const value of [null,{}, {key:'tiny'}, {key:42},{key:'abcdefgh',id:5},{key:'abcdefgh',id:''},{key:'a'.repeat(201)}]){assert.equal(restoredInvoiceAttempt(value),undefined);checks++;}
console.log(`${checks} payment state and reload recovery assertions passed`);
