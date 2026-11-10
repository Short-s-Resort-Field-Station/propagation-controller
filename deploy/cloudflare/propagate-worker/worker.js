// Reverse proxy: shortsfieldstation.org/propagate/* -> the unit's tunnel.
// The app is built with base "/propagate/" and the Go server mounts itself
// under that prefix, so paths pass through unchanged. Streams SSE, forwards
// cookies (session auth lives in the app), and marks everything noindex —
// this is an unlisted page, not part of the public site.
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === "/propagate") {
      return Response.redirect(`${url.origin}/propagate/`, 302);
    }
    const upstream = new URL(url.pathname + url.search, env.ORIGIN);
    const headers = new Headers(request.headers);
    headers.set("Host", new URL(env.ORIGIN).host);
    headers.set("X-Forwarded-Host", url.host);
    headers.set("X-Forwarded-Proto", "https");
    // Never let the origin's loopback-admin shortcut apply to tunnel traffic.
    headers.set("X-Forwarded-For", request.headers.get("CF-Connecting-IP") || "0.0.0.0");

    const resp = await fetch(upstream, {
      method: request.method,
      headers,
      body: ["GET", "HEAD"].includes(request.method) ? undefined : request.body,
      redirect: "manual",
    });
    const out = new Headers(resp.headers);
    out.set("X-Robots-Tag", "noindex, nofollow");
    out.delete("Content-Security-Policy"); // the Pages _headers CSP is not ours
    return new Response(resp.body, { status: resp.status, headers: out });
  },
};
