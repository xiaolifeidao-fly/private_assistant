/** next.config.mjs 里的 basePath。router / <Link> 会自动带上，fetch、<img>、metadata 里的地址要自己拼。 */
export const basePath = process.env.NEXT_PUBLIC_BASE_PATH || "";

export const withBase = (path: string) => `${basePath}${path}`;
