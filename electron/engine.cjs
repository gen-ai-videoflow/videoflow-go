const {spawn} = require('node:child_process');

class Engine {
  constructor({executable, args = ['--desktop', '--no-browser'], env = {}, timeout = 30000, request = fetch, spawnProcess = spawn}) {
    Object.assign(this, {executable, args, env, timeout, request, spawnProcess});
    this.stopping = false;
  }
  async start() {
    if (this.child || this.stopping) throw new Error('Engine already started or stopped');
    const environment = {...process.env, ...this.env, PORT: '0'};
    delete environment.NODE_TEST_CONTEXT;
    this.child = this.spawnProcess(this.executable, this.args, {
      env: environment, stdio: ['ignore', 'pipe', 'pipe']
    });
    // Attach the exit promise before either an error or an early exit can occur.
    this.exited = new Promise(resolve => {
      this.child.once('exit', resolve);
      this.child.once('error', resolve);
    });
    const url = await new Promise((resolve, reject) => {
      let done = false;
      const timer = setTimeout(() => finish(new Error('6eek engine startup timed out')), this.timeout);
      const finish = (error, value) => {
        if (done) return;
        done = true;
        clearTimeout(timer);
        this.abortStart = null;
        error ? reject(error) : resolve(value);
      };
      this.abortStart = () => finish(new Error('6eek engine stopped during startup'));
      this.child.once('error', err => finish(err));
      this.child.once('exit', code => finish(new Error(`6eek engine exited (${code})`)));
      for (const stream of [this.child.stdout, this.child.stderr]) {
        let buffer = '';
        stream.on('data', chunk => {
          buffer = (buffer + chunk.toString()).slice(-8192);
          const match = buffer.match(/6eek ready at (http:\/\/127\.0\.0\.1:[1-9]\d*)\r?\n/);
          if (match) finish(null, match[1]);
        });
      }
    }).catch(async error => { await this.stop(); throw error; });
    try {
      const response = await this.request(url + '/api/health', {signal: AbortSignal.timeout(10000)});
      const health = await response.json();
      if (!response.ok || health.status !== 'ok' || health.server !== '6eek-go-v1' || !health.media_ready) {
        throw new Error('Bundled 6eek engine or media tools failed their health check');
      }
      if (this.stopping || this.child.exitCode !== null || this.child.signalCode !== null) {
        throw new Error('6eek engine stopped during startup');
      }
      return url;
    } catch (error) { await this.stop(); throw error; }
  }
  stop() {
    if (this.stopPromise) return this.stopPromise;
    this.stopping = true;
    this.abortStart?.();
    this.stopPromise = (async () => {
      if (!this.child || this.child.exitCode !== null || this.child.signalCode !== null) return;
      this.child.kill('SIGTERM');
      const timer = setTimeout(() => this.child.kill('SIGKILL'), 15000);
      try { await this.exited; } finally { clearTimeout(timer); }
    })();
    return this.stopPromise;
  }
}
module.exports = {Engine};
