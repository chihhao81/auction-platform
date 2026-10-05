import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  async rewrites() {
    const backendOrigin = process.env.BACKEND_ORIGIN;
    if (!backendOrigin) return [];

    return [
      {
        source: "/v1/:path*",
        destination: `${backendOrigin.replace(/\/$/, "")}/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;

