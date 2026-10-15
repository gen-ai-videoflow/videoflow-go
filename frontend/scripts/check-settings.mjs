import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';import ts from 'typescript';
const code=ts.transpileModule(readFileSync(new URL('../src/lib/cloud/navigation.ts',import.meta.url),'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText;
const {settingsSection,canonicalSettingsPath}=await import('data:text/javascript;base64,'+Buffer.from(code).toString('base64'));
for(const [path,section] of [['/settings','account'],['/settings/security','security'],['/profile','profile'],['/security','security'],['/payments','billing'],['/settings/playback','playback']]){assert.equal(settingsSection(path),section);assert.equal(canonicalSettingsPath(path),`/settings/${section}`);}
assert.equal(settingsSection('/studio'),null);assert.equal(settingsSection('/settings/nope'),'unknown');
console.log('Settings canonical and legacy navigation checks passed');
