import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

// A same-origin gateway keeps session cookies first-party in the Node deployment.
const proxy: RequestHandler = async ({ request, params, url }) => {
  const base = env.CLOUD_API_INTERNAL_URL;
  if (import.meta.env.VITE_CLOUD_MODE !== 'true' || !base) {
    return Response.json({ code: 'api_unconfigured', message: 'The cloud API connection is not configured.' }, { status: 503 });
  }
  const headers = new Headers(request.headers);
  for (const header of ['host','connection','content-length','transfer-encoding']) headers.delete(header);
  const target = `${base.replace(/\/$/, '')}/${params.path || ''}${url.search}`;
  try {
    const response = await fetch(target, {
      method: request.method,
      headers,
      body: ['GET','HEAD'].includes(request.method) ? undefined : await request.arrayBuffer(),
      redirect: 'manual'
    });
    const responseHeaders = new Headers(response.headers);
    for (const header of ['connection','transfer-encoding','content-encoding','content-length']) responseHeaders.delete(header);
    responseHeaders.set('cache-control','no-store');
    return new Response(response.body, {status:response.status,headers:responseHeaders});
  } catch {
    return Response.json({code:'api_unavailable',message:'The studio is temporarily unavailable. Please try again.'},{status:502});
  }
};
export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
export const PUT = proxy;
export const DELETE = proxy;
