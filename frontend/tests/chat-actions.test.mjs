import test from 'node:test';
import assert from 'node:assert/strict';
import {normalizeChatIntent} from '../src/lib/chat/intent.mjs';
test('legacy saved scene cards become runnable script cards without losing their idea',()=>{
 const old={action:'add_scene',idea:'猫和孩子捉迷藏',scene_count:3};
 assert.deepEqual(normalizeChatIntent(old),{...old,action:'generate_script'});
 assert.equal(old.action,'add_scene');
 assert.equal(normalizeChatIntent({action:'create_script'}).action,'generate_script');
});
test('unsupported actions use the help card instead of a disabled Run button',()=>{
 for(const action of ['none','invented_action','',null])assert.equal(normalizeChatIntent({action}).action,'unknown');
 assert.equal(normalizeChatIntent({action:'generate_shots'}).action,'generate_shots');
});
