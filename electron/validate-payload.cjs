const fs = require('node:fs');
const path = require('node:path');
module.exports = async context => {
  const {Arch} = require('builder-util');
  const payload = JSON.parse(fs.readFileSync(path.join(__dirname, 'payload', '6eek', 'target.json'), 'utf8'));
  const platform = context.electronPlatformName;
  const arch = Arch[context.arch];
  if (payload.platform !== platform || payload.arch !== arch) {
    throw new Error(`Engine payload is ${payload.platform}/${payload.arch}, installer target is ${platform}/${arch}. Prepare the matching native archive first.`);
  }
};
