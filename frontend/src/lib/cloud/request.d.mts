export class CloudError extends Error {status:number;retryAfter:number;constructor(message:string,status:number,retryAfter?:number);}
export function fetchJSON(url:string,init?:RequestInit,options?:{signal?:AbortSignal;timeoutMs?:number;fetch?:typeof fetch}):Promise<any>;
