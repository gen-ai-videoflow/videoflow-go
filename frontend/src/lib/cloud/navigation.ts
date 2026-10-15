const legacy:Record<string,string>={'/settings':'account','/profile':'profile','/security':'security','/payments':'billing'};
const sections=['account','profile','security','playback','privacy','billing'];
export function settingsSection(path:string):string|null {if(legacy[path])return legacy[path];if(!path.startsWith('/settings/'))return null;const section=path.slice('/settings/'.length);return sections.includes(section)?section:'unknown';}
export function canonicalSettingsPath(path:string):string {const section=settingsSection(path);return section&&section!=='unknown'?`/settings/${section}`:path;}
