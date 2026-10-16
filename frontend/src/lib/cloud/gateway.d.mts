export function proxyRequest(request:Request,target:string,options?:{maxBytes?:number;timeoutMs?:number;maxConcurrent?:number;fetch?:typeof fetch}):Promise<Response>;
