// Chrome DevTools 打开时会自动请求此路径；返回空响应，避免 dev 日志刷 404。
export function GET() {
  return new Response(null, { status: 204 });
}
