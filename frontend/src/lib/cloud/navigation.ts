const legacy:Record<string,string>={'/settings':'account','/profile':'profile','/security':'security','/payments':'billing'};
const sections=['account','profile','security','playback','privacy','billing'];
export function settingsSection(path:string):string|null {if(legacy[path])return legacy[path];if(!path.startsWith('/settings/'))return null;const section=path.slice('/settings/'.length);return sections.includes(section)?section:'unknown';}
export function canonicalSettingsPath(path:string):string {const section=settingsSection(path);return section&&section!=='unknown'?`/settings/${section}`:path;}

const studioSections=['storyboard','scripts','characters','scenes','assets','shots','captions'];
export function studioSection(path:string,tab:string):string|null {
 if(!['/studio','/workspace','/scenes','/characters','/assets'].includes(path))return null;
 if(studioSections.includes(tab))return tab;
 if(['/scenes','/characters','/assets'].includes(path))return path.slice(1);
 return path==='/workspace'?'scripts':null;
}
export function selectStudioProject(projects:{id:string}[],requested:string|null,saved:string|null):string {
 return projects.find(p=>p.id===requested)?.id||projects.find(p=>p.id===saved)?.id||projects[0]?.id||'';
}
export function studioHref(kind:string,project:string):string {
 const path=['characters','scenes','assets'].includes(kind)?'/'+kind:'/workspace';
 const query=new URLSearchParams({...(['characters','scenes','assets'].includes(kind)?{}:{tab:kind}),...(project?{project}:{})});
 return path+'?'+query.toString();
}
