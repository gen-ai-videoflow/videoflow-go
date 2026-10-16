import { fetchJSON } from './request.mjs';
export { CloudError } from './request.mjs';
const base = (import.meta.env.VITE_CLOUD_API_BASE || '/api/v1').replace(/\/$/, '');
export async function request(path: string, body?: unknown, method?: string, headers: Record<string,string> = {}, keepalive = false, options: {signal?:AbortSignal;timeoutMs?:number} = {}) {
  return fetchJSON(`${base}${path}`, { credentials: 'include', keepalive, method: method || (body === undefined ? 'GET' : 'POST'), headers: { ...(body === undefined ? {} : {'Content-Type': 'application/json'}), ...headers }, body: body === undefined ? undefined : JSON.stringify(body) }, options);
}
