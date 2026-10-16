const {test} = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const {Engine} = require('../engine.cjs');
const fixture = path.join(__dirname, 'fixture.cjs');
test('starts on an assigned loopback port and stops its process', {skip: !process.env.SIXEEK_INTEGRATION}, async () => {
  const engine = new Engine({executable: process.execPath, args: [fixture], timeout: 3000});
  try {
    const url = await engine.start();
    assert.match(url, /^http:\/\/127\.0\.0\.1:[1-9]\d*$/);
    assert.equal((await (await fetch(url + '/api/health')).json()).media_ready, true);
  } finally { await engine.stop(); }
  assert.notEqual(engine.child.exitCode, null);
});
test('startup failure rejects and leaves no running child', async () => {
  const engine = new Engine({executable: process.execPath, args: [fixture], env: {FIXTURE_FAIL: '1'}, timeout: 3000});
  await assert.rejects(engine.start(), /exited/);
  await engine.stop();
  assert.equal(engine.child.exitCode, 2);
});
test('stopping during startup aborts readiness', async () => {
  const engine = new Engine({executable: process.execPath, args: [fixture], timeout: 3000});
  const started = engine.start();
  const rejected = assert.rejects(started, /stopped|exited/);
  await engine.stop();
  await rejected;
});


const {EventEmitter} = require('node:events');
function fakeSpawn(_command, _args, options) {
  assert.equal(options.env.PORT, '0');
  const child = new EventEmitter();
  Object.assign(child, {stdout: new EventEmitter(), stderr: new EventEmitter(), exitCode: null, signalCode: null});
  child.kill = signal => { child.signalCode = signal; setImmediate(() => child.emit('exit', null, signal)); return true; };
  setImmediate(() => {
    child.stderr.emit('data', Buffer.from('6eek ready at http://127.'));
    child.stdout.emit('data', Buffer.from('unrelated log\n'));
    child.stderr.emit('data', Buffer.from('0.0.1:12345\n'));
  });
  return child;
}
test('fragmented readiness, health verification and graceful shutdown', async () => {
  const engine = new Engine({executable: 'fixture', spawnProcess: fakeSpawn,
    request: async url => { assert.equal(url, 'http://127.0.0.1:12345/api/health');
      return {ok: true, json: async () => ({status: 'ok', server: '6eek-go-v1', media_ready: true})}; }});
  await engine.start();
  await engine.stop();
  assert.equal(engine.child.signalCode, 'SIGTERM');
});
test('failed media health stops the engine', async () => {
  const engine = new Engine({executable: 'fixture', spawnProcess: fakeSpawn,
    request: async () => ({ok: true, json: async () => ({status: 'ok', server: '6eek-go-v1', media_ready: false})})});
  await assert.rejects(engine.start(), /health check/);
  assert.equal(engine.child.signalCode, 'SIGTERM');
});
test('missing executable reports startup error without waiting for shutdown timeout', async () => {
  const engine = new Engine({executable: '/nonexistent/6eek-engine'});
  await assert.rejects(engine.start(), /ENOENT/);
});
