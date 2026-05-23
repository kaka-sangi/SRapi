const DEFAULT_API_TARGET = 'http://127.0.0.1:8080';
const HEALTH_TIMEOUT_MS = 2500;

export const dynamic = 'force-dynamic';

export async function GET() {
  const apiTarget = (process.env.SRAPI_API_PROXY_TARGET || DEFAULT_API_TARGET).replace(/\/+$/, '');
  const controller = new AbortController();
  const timeout = globalThis.setTimeout(() => controller.abort(), HEALTH_TIMEOUT_MS);

  try {
    const response = await fetch(`${apiTarget}/api/v1/health`, {
      cache: 'no-store',
      signal: controller.signal
    });
    const body = await response.text();

    return new Response(body || '{}', {
      status: response.ok ? 200 : 503,
      headers: {
        'Content-Type': response.headers.get('Content-Type') || 'application/json'
      }
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : 'backend unavailable';
    return Response.json(
      {
        data: {
          status: 'unavailable',
          target: apiTarget
        },
        error: message
      },
      { status: 503 }
    );
  } finally {
    globalThis.clearTimeout(timeout);
  }
}
