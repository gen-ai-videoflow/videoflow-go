export function promptBytes(text:string):number;
export function variableNames(text:string):string[];
export function renderPrompt(text:string,values:Record<string,string>):{text:string;bytes:number;missing:string[];invalid:boolean};
