export function appendUnique<T extends {id:string}>(current:T[],next:T[]):T[]{const seen=new Set(current.map(v=>v.id));return [...current,...next.filter(v=>!seen.has(v.id)&&!!seen.add(v.id))];}
export function adjacentIndex(index:number,change:number,length:number):number{return Math.max(0,Math.min(Math.max(0,length-1),index+change));}
export function requestGate(){let sequence=0;return {next:()=>++sequence,current:(value:number)=>value===sequence};}
