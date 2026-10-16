import assert from 'node:assert/strict';
import {historyQuery,mergeHistoryEntries} from '../src/lib/cloud/billing-history.mjs';
assert.equal(historyQuery('stripe','premium','cursor').get('provider'),'stripe');
assert.equal(historyQuery('all','all','').has('cursor'),false);
assert.throws(()=>historyQuery('unknown','all',''));
assert.throws(()=>historyQuery('all','unknown',''));
assert.deepEqual(mergeHistoryEntries([{id:'one'}],[{id:'one'},{id:'two'}]).map(x=>x.id),['one','two']);
console.log('Billing history filters, cursor and duplicate page recovery passed');
