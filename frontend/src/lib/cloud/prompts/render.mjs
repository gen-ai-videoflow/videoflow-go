export const promptBytes = text => new TextEncoder().encode(text).length;
export const variableNames = text => [...new Set([...text.matchAll(/\{\{\s*([a-z][a-z0-9_]{0,39})\s*\}\}/g)].map(match => match[1]))];
export function renderPrompt(text,values){
 const missing=variableNames(text).filter(key=>!Object.hasOwn(values,key)||!String(values[key]??'').trim());
 const rendered=text.replace(/\{\{\s*([a-z][a-z0-9_]{0,39})\s*\}\}/g,(_,key)=>Object.hasOwn(values,key)?String(values[key]??'').trim():'');
 return {text:rendered,bytes:promptBytes(rendered),missing,invalid:/\{\{|\}\}/.test(rendered)};
}
