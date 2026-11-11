import { canvasRequest } from './cloud/legacy';
const cloudMode = import.meta.env.VITE_CLOUD_MODE === 'true';
/** HTTP client for the Go backend. */

// Follow the page's hostname (localhost vs 127.0.0.1 are different origins —
// using the same host keeps CORS consistent however the app was opened).
export const API_BASE =
  import.meta.env.VITE_API_BASE ??
  (typeof window !== 'undefined'
    ? (['5173', '4173'].includes(window.location.port)
      ? `http://${window.location.hostname}:8000`
      : `${window.location.origin}/api`)
    : 'http://localhost:8000');

async function handle(resp: Response) {
  if (!resp.ok) {
    const text = await resp.text();
    let detail: unknown = text || resp.statusText;
    try { const body=JSON.parse(text);detail=body.detail ?? body.error ?? JSON.stringify(body); } catch { /* plain-text backend error is already useful */ }
    throw new Error(`${resp.status}: ${formatDetail(detail)}`);
  }
  return resp.json();
}

function formatDetail(detail: unknown): string {
  if (Array.isArray(detail)) {
    return detail.map((item: any) => {
      const loc = Array.isArray(item?.loc)
        ? item.loc.filter((part: unknown) => part !== 'body').map(String).join('.')
        : '';
      const message = typeof item?.msg === 'string' ? item.msg : JSON.stringify(item);
      return loc ? `${loc}: ${message}` : message;
    }).join('; ');
  }
  if (detail && typeof detail === 'object') return JSON.stringify(detail);
  return String(detail);
}

export const get = (path: string) => cloudMode ? canvasRequest(path) : fetch(`${API_BASE}${path}`).then(handle);

// Keep the same request key after a lost response, including page reloads.
async function localPost(path:string,body?:unknown){
 const payload=body===undefined?undefined:JSON.stringify(body);
 const bytes=new TextEncoder().encode(path+':'+(payload??''));
 let fingerprint:string;
 if(crypto.subtle){const hash=await crypto.subtle.digest('SHA-256',bytes);fingerprint=Array.from(new Uint8Array(hash),b=>b.toString(16).padStart(2,'0')).join('')}
 else{let hash=14695981039346656037n;for(const b of bytes)hash=BigInt.asUintN(64,(hash^BigInt(b))*1099511628211n);fingerprint=hash.toString(16)}
 const slot='6eek.pending.'+fingerprint;
 const newKey=()=>{const bytes=crypto.getRandomValues(new Uint8Array(16));return Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('')};
 let key:string;try{key=sessionStorage.getItem(slot)||newKey();sessionStorage.setItem(slot,key)}catch{key=newKey()}
 const response=await fetch(`${API_BASE}${path}`,{method:'POST',headers:{...(payload===undefined?{}:{'Content-Type':'application/json'}),'Idempotency-Key':key},body:payload});
 try{sessionStorage.removeItem(slot)}catch{/* storage may be unavailable */}
 return handle(response);
}
export const post = (path:string,body?:unknown)=>cloudMode?canvasRequest(path,body,'POST'):localPost(path,body);

export const put = (path: string, body: unknown) =>
  cloudMode ? canvasRequest(path, body, 'PUT') : fetch(`${API_BASE}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(handle);

export const patch = (path: string, body: unknown) =>
  cloudMode ? canvasRequest(path, body, 'PATCH') : fetch(`${API_BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(handle);

export const del = (path: string) =>
  cloudMode ? canvasRequest(path, {}, 'DELETE') : fetch(`${API_BASE}${path}`, { method: 'DELETE' }).then(handle);

export const upload = (path: string, form: FormData) =>
  cloudMode ? Promise.reject(new Error('Use the cloud asset upload workflow.')) : fetch(`${API_BASE}${path}`, { method: 'POST', body: form }).then(handle);

/** Map a stored file_path (absolute or relative) onto the /storage mount. */
export function mediaUrl(filePath: string | null | undefined): string | null {
  if (cloudMode || !filePath) return null;
  const i = filePath.indexOf('storage/');
  if (i < 0) return null;
  return `${API_BASE}/${filePath.slice(i)}`;
}

export const isImage = (p: string | null | undefined) =>
  !!p && /\.(png|jpe?g|webp|gif)$/i.test(p);
export const isVideo = (p: string | null | undefined) =>
  !!p && /\.(mp4|webm|mov)$/i.test(p);
