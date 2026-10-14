/**
 * /api 全部转给 manager-api。开发时默认本机 10013；部署时用 MANAGER_API 指过去，
 * 或者由 nginx 直接把 /api 转给后端（那样这里的 rewrite 不会被走到）。
 */
const managerApi = process.env.MANAGER_API || "http://127.0.0.1:10013";

/**
 * 访问前缀，页面、静态资源和 /api 都挂在它下面（nginx 按前缀转发即可）。
 * 构建期定死；换前缀要重新构建：BASE_PATH=/xxx ./build.sh，设成空字符串就是挂在根路径。
 */
const basePath = process.env.BASE_PATH ?? "/assistant-manager";

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  basePath,
  // 代码里拼 fetch / <img> / Service Worker 地址时要用到
  env: { NEXT_PUBLIC_BASE_PATH: basePath },
  reactStrictMode: false,
  // rewrite 代理默认 30 秒没有数据就断开；人设卡生成、模型思考可能更久
  experimental: { proxyTimeout: 600_000 },
  async redirects() {
    // 直接打开根路径时跳到前缀下，免得看到 404
    return basePath ? [{ source: "/", destination: basePath, basePath: false, permanent: false }] : [];
  },
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${managerApi}/api/:path*` }];
  },
};

export default nextConfig;
