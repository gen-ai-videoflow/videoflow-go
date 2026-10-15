import staticAdapter from '@sveltejs/adapter-static';
import nodeAdapter from '@sveltejs/adapter-node';
const cloudMode = process.env.VITE_CLOUD_MODE === 'true';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    // Preserve desktop SPA packaging; cloud builds use Node for public SSR and the API gateway.
    adapter: cloudMode ? nodeAdapter() : staticAdapter({ fallback: 'index.html' })
  }
};

export default config;
