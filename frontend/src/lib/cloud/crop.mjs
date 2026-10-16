export function cropGeometry(sourceWidth,sourceHeight,width,height,zoom,x,y){
 if(![sourceWidth,sourceHeight,width,height,zoom,x,y].every(Number.isFinite)||sourceWidth<=0||sourceHeight<=0||width<=0||height<=0)throw new Error('Invalid image dimensions');
 const scale=Math.max(width/sourceWidth,height/sourceHeight)*Math.max(1,Math.min(3,zoom));
 const scaledWidth=sourceWidth*scale,scaledHeight=sourceHeight*scale;
 return{width:scaledWidth,height:scaledHeight,x:(width-scaledWidth)*Math.max(0,Math.min(100,x))/100,y:(height-scaledHeight)*Math.max(0,Math.min(100,y))/100};
}
