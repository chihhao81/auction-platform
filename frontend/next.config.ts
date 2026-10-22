import type { NextConfig } from "next";

// The browser only ever talks to this Next.js origin. /api and /uploads are
// proxied to the Go backend, so the session cookie is first-party and no CORS
// configuration is needed.
const backendURL = process.env.BACKEND_URL ?? "http://localhost:8080";
const isDev = process.env.NODE_ENV === "development";

// Content Security Policy. Pages are statically prerendered, so the
// "without nonces" policy from the Next.js docs is used. Image hosts: LINE
// avatars and, in production, the R2 public image origin.
const imageOrigin = process.env.NEXT_PUBLIC_IMAGE_ORIGIN ?? "";
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline'${isDev ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline'",
  `img-src 'self' blob: data: https://profile.line-scdn.net https://obs.line-scdn.net ${imageOrigin}`.trim(),
  "font-src 'self'",
  "connect-src 'self'",
  "worker-src 'self'",
  "object-src 'none'",
  "base-uri 'self'",
  // The only cross-site form post is the hand-off to ECPay.
  "form-action 'self' https://payment.ecpay.com.tw https://payment-stage.ecpay.com.tw",
  "frame-ancestors 'none'",
  ...(isDev ? [] : ["upgrade-insecure-requests"]),
].join("; ");

const nextConfig: NextConfig = {
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${backendURL}/api/:path*` },
      { source: "/uploads/:path*", destination: `${backendURL}/uploads/:path*` },
    ];
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Content-Security-Policy", value: csp },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
        ],
      },
    ];
  },
};

export default nextConfig;
