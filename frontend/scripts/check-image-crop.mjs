import assert from 'node:assert/strict';import{cropGeometry}from'../src/lib/cloud/crop.mjs';
assert.deepEqual(cropGeometry(1000,500,512,512,1,50,50),{width:1024,height:512,x:-256,y:0});
for(const x of [-100,0,50,100,200]){const g=cropGeometry(1000,500,512,512,2,x,50);assert.ok(g.x<=0&&g.x+g.width>=512&&g.y<=0&&g.y+g.height>=512);}
assert.throws(()=>cropGeometry(0,10,512,512,1,50,50));console.log('Image crop keeps selected frame fully covered and bounds positioning');
