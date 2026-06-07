import type { NextRequest } from "next/server";
import { proxyToBackend } from "@/lib/backend-proxy";

// Node runtime is required for getSetCookie() + streaming request bodies
// (duplex:'half'). force-dynamic prevents any caching of the proxy.
export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function handler(req: NextRequest): Promise<Response> {
  return proxyToBackend(req);
}

export {
  handler as GET,
  handler as POST,
  handler as PUT,
  handler as PATCH,
  handler as DELETE,
  handler as OPTIONS,
  handler as HEAD,
};
