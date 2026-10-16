const {app, BrowserWindow, dialog, shell} = require('electron');
const path = require('node:path');
const {Engine} = require('./engine.cjs');
const smokeTest = process.argv.includes('--smoke-test');
let engine, window, quitting = false;
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on('second-instance', () => { if (window) { if (window.isMinimized()) window.restore(); window.focus(); } });
  app.on('window-all-closed', () => app.quit());
  app.on('before-quit', event => {
    if (quitting) return;
    event.preventDefault();
    quitting = true;
    Promise.resolve(engine?.stop()).finally(() => app.quit());
  });
  app.whenReady().then(async () => {
    const folder = app.isPackaged ? path.join(process.resourcesPath, 'engine') : path.join(__dirname, 'payload', '6eek');
    engine = new Engine({executable: path.join(folder, '6eek')});
    try {
      const url = await engine.start();
      if (quitting) return;
      engine.child.once('exit', () => {
        if (!quitting) {
          if (smokeTest) { console.error('Engine exited during smoke test'); process.exitCode = 1; }
          else dialog.showErrorBox('6eek stopped', 'The local engine stopped. Reopen 6eek to continue.');
          app.quit();
        }
      });
      window = new BrowserWindow({width: 1280, height: 860, minWidth: 800, minHeight: 600, show: false,
        webPreferences: {nodeIntegration: false, contextIsolation: true, sandbox: true}});
      window.webContents.session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
      window.webContents.session.setPermissionCheckHandler(() => false);
      const external = value => {
        try { const target = new URL(value); if (['https:', 'http:'].includes(target.protocol)) void shell.openExternal(value); } catch {}
      };
      window.webContents.setWindowOpenHandler(({url: target}) => { external(target); return {action: 'deny'}; });
      window.webContents.on('will-navigate', (event, target) => {
        if (new URL(target).origin !== url) { event.preventDefault(); external(target); }
      });
      window.webContents.on('will-redirect', (event, target) => { if (new URL(target).origin !== url) event.preventDefault(); });
      window.once('ready-to-show', () => { if (!quitting && !smokeTest) window.show(); });
      await window.loadURL(url);
      if (smokeTest) {
        const rendered = await window.webContents.executeJavaScript(`new Promise(resolve => {
          const deadline = Date.now() + 10000;
          const check = () => {
            if (document.body.textContent.includes('6eek')) resolve(true);
            else if (Date.now() > deadline) resolve(false);
            else setTimeout(check, 100);
          };
          check();
        })`);
        if (!rendered) throw new Error('6eek interface did not render');
        console.log(JSON.stringify({desktop_ready: true, url, engine_pid: engine.child.pid}));
        app.quit();
      }
    } catch (error) {
      if (smokeTest) { console.error(error.message); process.exitCode = 1; }
      else if (!quitting) dialog.showErrorBox('6eek could not start', String(error.message));
      app.quit();
    }
  });
}
