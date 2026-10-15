const base = (import.meta.env.VITE_CLOUD_API_BASE || '/api/v1').replace(/\/$/, '');
export class CloudError extends Error { constructor(message: string, public status: number) { super(message); } }
export async function request(path: string, body?: unknown, method?: string, headers: Record<string,string> = {}, keepalive = false) {
  const response = await fetch(`${base}${path}`, { credentials: 'include', keepalive, method: method || (body === undefined ? 'GET' : 'POST'), headers: { ...(body === undefined ? {} : {'Content-Type': 'application/json'}), ...headers }, body: body === undefined ? undefined : JSON.stringify(body) });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new CloudError(data.message || `Request failed (${response.status}). Please try again.`, response.status);
  return data;
}
