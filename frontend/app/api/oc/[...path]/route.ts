import { NextRequest } from "next/server";

type RouteContext = { params: Promise<{ path: string[] }> };

async function forward(request: NextRequest, context: RouteContext): Promise<Response> {
  const baseURL = process.env.OC_API_URL ?? "http://localhost:8080";
  const { path } = await context.params;
  const target = new URL(path.join("/"), `${baseURL.replace(/\/$/, "")}/`);
  target.search = request.nextUrl.search;

  const headers = new Headers();
  for (const name of ["accept", "content-type", "x-request-id"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }

  const hasBody = request.method !== "GET" && request.method !== "HEAD";

  try {
    const upstream = await fetch(target, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      cache: "no-store",
      redirect: "manual",
    });

    const responseHeaders = new Headers();
    const contentType = upstream.headers.get("content-type");
    if (contentType) responseHeaders.set("content-type", contentType);
    const location = upstream.headers.get("location");
    if (location) {
      responseHeaders.set("location", location.startsWith("/") ? `/api/oc${location}` : location);
    }

    return new Response(upstream.body, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error("OC API proxy failed", error);
    return Response.json(
      {
        status: 502,
        title: "Bad Gateway",
        detail: "The claims service is unavailable. Check that the Go API is running.",
      },
      { status: 502 },
    );
  }
}

export const dynamic = "force-dynamic";

export const GET = forward;
export const POST = forward;
