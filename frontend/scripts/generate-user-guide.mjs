import fs from 'node:fs';
const source=new URL('../src/lib/cloud/help/guide.json',import.meta.url);
const sections=JSON.parse(fs.readFileSync(source,'utf8'));
const ids=new Set();
for(const section of sections){if(ids.has(section.id)||!section.title||!section.lead||!section.steps.length)throw new Error('Invalid guide section');ids.add(section.id);for(const link of section.links)if(!link.href.startsWith('/')||link.href.startsWith('//'))throw new Error('Guide links must remain within the application');}
const content='# VideoFlow cloud user guide\n\nFrom your first account to watching, creating, editing and publishing.\n\nThis document is generated from the same content as the in-app Help guide. It covers the cloud web application, not the desktop handbook.\n\n'+sections.map(s=>`## ${s.title}\n\n${s.lead}\n\n${s.steps.map((x,i)=>`${i+1}. ${x}`).join('\n\n')}\n\n${s.notes.map(x=>`- ${x}`).join('\n\n')}\n`).join('\n');
const target=new URL('../static/help/user-guide.md',import.meta.url);
if(process.argv.includes('--check')){if(fs.readFileSync(target,'utf8')!==content)throw new Error('Regenerate the user guide');}else{fs.mkdirSync(new URL('../static/help/',import.meta.url),{recursive:true});fs.writeFileSync(target,content);}
console.log(`Cloud user guide: ${sections.length} verified sections`);
