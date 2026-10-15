import {spawnSync} from 'node:child_process';import{existsSync,mkdirSync,copyFileSync,readFileSync,writeFileSync,readdirSync}from'node:fs';
if(process.env.VITE_CLOUD_MODE!=='true')process.exit(0);
if(!existsSync('editor/node_modules')){const r=spawnSync('npm',['ci','--prefix','editor','--ignore-scripts'],{stdio:'inherit'});if(r.status)process.exit(r.status);}
const result=spawnSync('npm',['run','build','--prefix','editor'],{stdio:'inherit'});if(result.status)process.exit(result.status);
let notices='VideoFlow Editor incorporates OpenCut Classic.\n\n'+readFileSync('editor/LICENSE-OPENCUT','utf8')+'\n\nDependency source versions and rebuild instructions: editor/package-lock.json and editor/README.md in the VideoFlow Web source distribution.\n';
const lock=JSON.parse(readFileSync('editor/package-lock.json','utf8'));
for(const name of Object.keys(lock.packages).filter(p=>p.startsWith('node_modules/'))){const dir='editor/'+name;if(!existsSync(dir))continue;for(const filename of readdirSync(dir).filter(n=>/^(license|copying|notice)(\.|$)/i.test(n))){try{notices+=`\n\n--- ${name} ---\n`+readFileSync(dir+'/'+filename,'utf8');}catch{}}}
writeFileSync('static/editor-app/notices.txt',notices);
mkdirSync('static/editor-app/sources',{recursive:true});
for(const name of ['mediabunny','soundtouchjs']){const result=spawnSync('tar',['-czf',`static/editor-app/sources/${name}.tar.gz`,'-C','editor/node_modules',name],{stdio:'inherit'});if(result.status)process.exit(result.status);}
copyFileSync('editor/package-lock.json','static/editor-app/sources/package-lock.json');copyFileSync('editor/README.md','static/editor-app/sources/BUILD.md');
