/** Only root-relative destinations on this origin may follow authentication. */
export function safeReturnTo(value: string | null | undefined, origin: string): string {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return '/discover';
  try {
    let inspected = value;
    for (let i = 0; i < 4; i++) {
      if (/[\\\u0000-\u001f\u007f]/.test(inspected) || inspected.startsWith('//')) return '/discover';
      const decoded = decodeURIComponent(inspected);
      if (decoded === inspected) break;
      inspected = decoded;
    }
    if (/[\\\u0000-\u001f\u007f]/.test(inspected) || inspected.startsWith('//')) return '/discover';
    const base = new URL(origin);
    const target = new URL(value, base);
    const decodedTarget = new URL(inspected, base);
    for (const url of [target, decodedTarget]) {
      if (url.origin !== base.origin || url.pathname.startsWith('//') || ['/login', '/register', '/forgot-password', '/reset-password', '/verify-email'].includes(url.pathname)) return '/discover';
    }
    return target.pathname + target.search + target.hash;
  } catch {
    return '/discover';
  }
}

export function authHref(route: '/login' | '/register' | '/forgot-password', returnTo: string | null | undefined, origin: string): string {
  return `${route}?returnTo=${encodeURIComponent(safeReturnTo(returnTo, origin))}`;
}
