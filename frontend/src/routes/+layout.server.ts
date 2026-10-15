import { env } from '$env/dynamic/private';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ url, fetch, depends }) => {
  if (import.meta.env.VITE_CLOUD_MODE !== 'true') return {};
  depends('cloud:public');
  const path = url.pathname;
  let endpoint = path === '/discover' ? '/feed' : path.startsWith('/watch/') ? `/videos/${encodeURIComponent(path.split('/')[2])}` : path.startsWith('/channels/') ? `/channels/${encodeURIComponent(path.split('/')[2])}` : '';
  if (!endpoint) return {};
  const base = (env.CLOUD_API_INTERNAL_URL || import.meta.env.VITE_CLOUD_API_BASE || '/api/v1').replace(/\/$/, '');
  try {
    // Public metadata only. Session cookies and private balances stay out of SSR payloads.
    const response = await fetch(`${base}${endpoint}`, {credentials:'omit',headers:{accept:'application/json'}});
    if (!response.ok) return {};
    const value = await response.json();
    return { cloudPublic: path === '/discover' ? {videos:Array.isArray(value) ? value : value.items || value.videos || []} : value, cloudPath:path };
  } catch { return {}; }
};
