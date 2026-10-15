import { request } from './api';
let project = '';
export function getCanvasProject() { return project; }
export function canvasStorageKey() { return `videoflow.cloud.canvas.${project}`; }
export function setCanvasProject(id: string) { project = id; }
export async function canvasRequest(path: string, body?: unknown, method = 'GET') {
  if (typeof window === 'undefined' || !project) throw new Error('Select a project first.');
  const scope = `/projects/${encodeURIComponent(project)}`;
  if (path === '/graph') return request(scope + '/graph');
  if (/^\/(scenes\/[^/]+\/cast\/[^/]+|shots\/[^/]+\/assets\/[^/]+)$/.test(path)) return request(scope + path, method === 'GET' ? undefined : body || {}, method);
  if (/^\/(scenes|shots|characters|assets)(\/[^/]+)?$/.test(path)) return request(scope + '/canvas' + path, method === 'GET' ? undefined : body || {}, method);
  throw new Error('This action is not available in the cloud workspace yet. Use the credit-confirmed video workflow.');
}
