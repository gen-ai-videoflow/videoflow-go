// Cloud builds render public pages on the server; desktop remains a static SPA.
export const ssr = import.meta.env.VITE_CLOUD_MODE === 'true';
export const prerender = false;
