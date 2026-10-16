import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import {proxyRequest} from '$lib/cloud/gateway.mjs';
const proxy: RequestHandler = async ({ request, params, url }) => {
  const base = env.CLOUD_API_INTERNAL_URL;
  if (import.meta.env.VITE_CLOUD_MODE !== 'true' || !base) return Response.json({ code: 'api_unconfigured', message: 'The cloud API connection is not configured.' }, { status: 503 });
  return proxyRequest(request,`${base.replace(/\/$/, '')}/${params.path || ''}${url.search}`);
};
export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
export const PUT = proxy;
export const DELETE = proxy;
