"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { App } from "antd";

/**
 * 拉一次数据并提供 reload；出错时 error 里是后端返回的中文原因。
 * 筛选条件连着改时，只采用最后一次请求的结果，慢的旧请求回来也不会覆盖新数据。
 * 出错默认弹提示（否则表格只是空着，看不出是没数据还是没拉下来）；自己展示错误的页面传 silent。
 */
export function useData<T>(loader: () => Promise<T>, deps: unknown[] = [], options: { silent?: boolean } = {}) {
  const { message } = App.useApp();
  const silent = !!options.silent;
  const [data, setData] = useState<T | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const seq = useRef(0);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const load = useCallback(loader, deps);
  const reload = useCallback(async () => {
    const current = ++seq.current;
    setLoading(true);
    setError("");
    try {
      const result = await load();
      if (current === seq.current) setData(result);
    } catch (err) {
      if (current === seq.current) {
        setError((err as Error).message);
        // 401 已经在跳登录页了，不用再提示
        if (!silent && (err as { code?: number }).code !== 401) message.error((err as Error).message);
      }
    } finally {
      if (current === seq.current) setLoading(false);
    }
  }, [load, message, silent]);
  useEffect(() => {
    void reload();
  }, [reload]);
  return { data, loading, error, reload, setData };
}
