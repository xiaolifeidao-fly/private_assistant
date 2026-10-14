/**
 * /api 转给 app-api。开发时默认本机 10012；部署时用 APP_API 指过去，
 * 或者由 nginx 直接把 /api 转给后端（流式对话走 nginx 时要关 proxy_buffering）。
 */
const appApi = process.env.APP_API || "http://127.0.0.1:10012";

/**
 * 访问前缀，页面、静态资源和 /api 都挂在它下面（nginx 按前缀转发即可）。
 * 构建期定死；换前缀要重新构建：BASE_PATH=/xxx ./build.sh，设成空字符串就是挂在根路径。
 */
const basePath = process.env.BASE_PATH ?? "/assistant";

/**
 * 后端接口的前缀，对应后端 application.properties 的 server.context_path。
 * 默认和 basePath 一样；后端没配 context_path（接口直接在 /api）时，构建带上 API_CONTEXT_PATH= 置空。
 */
const apiContextPath = process.env.API_CONTEXT_PATH ?? basePath;

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  basePath,
  // 代码里拼 fetch / <img> / Service Worker 地址时要用到
  env: { NEXT_PUBLIC_BASE_PATH: basePath },
  reactStrictMode: false,
  // rewrite 代理默认 30 秒没有数据就断开；人设卡生成、模型思考可能更久
  experimental: { proxyTimeout: 600_000 },
  transpilePackages: ["antd-mobile"],
  async redirects() {
    // 直接打开根路径时跳到前缀下，免得看到 404
    return basePath ? [{ source: "/", destination: basePath, basePath: false, permanent: false }] : [];
  },
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${appApi}${apiContextPath}/api/:path*` }];
  },
  async headers() {
    return [
      // Service Worker 不能被缓存，否则更新发不下去。
      { source: "/sw.js", headers: [{ key: "Cache-Control", value: "no-cache" }] },
    ];
  },
};

export default nextConfig;
