let pending;
/** Memory-only transfer: no public blob URL, local-storage copy or automatic publication. */
export function stageExport(userId,{buffer,filename,mime}){
 if(!userId||!(buffer instanceof ArrayBuffer)||buffer.byteLength<1||buffer.byteLength>200*1024*1024||typeof filename!=='string'||filename.length>180||/[\\/\r\n\0]/.test(filename)||!['video/mp4','video/webm'].includes(mime))throw new Error('This export cannot be transferred. Download it and upload the saved file instead.');
 pending={userId,file:new File([buffer],filename,{type:mime}),created:Date.now()};
}
export function takeExport(userId){const item=pending;pending=undefined;return item?.userId===userId&&Date.now()-item.created<15*60*1000?item.file:null;}
