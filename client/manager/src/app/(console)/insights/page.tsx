"use client";

import { useState } from "react";
import dayjs from "dayjs";
import { App, Button, Empty, Spin, Table, Tabs } from "antd";
import { Copy, Sparkles } from "lucide-react";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { JobRun, Memory, MemoryMergeSuggestion, Paged } from "@/lib/types";
import { useConsole } from "@/components/console";
import { RichText } from "@/components/rich-text";
import { PageHeader, Panel, Pill } from "@/components/ui";

const jobLabel: Record<string, string> = { weekly_insight: "周洞察", merge_similar: "找重复" };

export default function InsightsPage() {
  return (
    <>
      <PageHeader
        eyebrow="记忆"
        title="每周整理"
        description="每周日晚上 8 点以后自动跑一次：总结这周她们在意什么（存成「归纳洞察」记忆并推送到手机），找出疑似重复的记忆交给你决定。在「对话规则 → 自动学习」里可以关掉。"
      />
      <Panel>
        <Tabs
          destroyInactiveTabPane
          items={[
            { key: "insight", label: "周洞察", children: <Insights /> },
            { key: "merge", label: "重复记忆", children: <Merges /> },
            { key: "runs", label: "执行记录", children: <Runs /> },
          ]}
        />
      </Panel>
    </>
  );
}

function Insights() {
  const { message } = App.useApp();
  const { personName } = useConsole();
  const { data, loading, reload } = useData(() => api.get<Paged<Memory>>("/memories?types=insight&size=30"), []);
  const [running, setRunning] = useState(false);
  const run = async () => {
    setRunning(true);
    try {
      const created = await api.post<Memory[] | null>("/insight/weekly");
      message.success(created?.length ? `生成了 ${created.length} 条洞察` : "这周没有新记忆，没有生成");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setRunning(false);
    }
  };
  return (
    <>
      <div className="pa-toolbar">
        <span className="pa-muted" style={{ fontSize: 13 }}>最近的洞察。它们也是记忆，助理检索时会用到，人设卡刷新时也会参考。</span>
        <div className="pa-toolbar-spacer" />
        <Button icon={<Sparkles size={15} />} className="pa-icon-text" loading={running} onClick={run}>现在生成一次</Button>
      </div>
      {loading && !data ? <div style={{ padding: 40, textAlign: "center" }}><Spin /></div> : !data?.items.length ? <Empty description="还没有洞察" /> : (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {data.items.map((item) => (
            <div key={item.memoryKey} style={{ border: "1px solid var(--pa-line)", borderRadius: 14, padding: "12px 16px" }}>
              <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 6 }}>
                <Pill tone="clay">{item.subjectPerson ? personName(item.subjectPerson) : "全家"}</Pill>
                <span style={{ fontWeight: 600 }}>{item.title}</span>
                <span style={{ flex: 1 }} />
                <span className="pa-num" style={{ fontSize: 12, color: "var(--pa-text-3)" }}>{dayjs(item.occurredAt ?? item.createdAt).format("YYYY-MM-DD")}</span>
              </div>
              <RichText text={item.content} />
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function MemoryCard({ memory, kept }: { memory: Memory | null; kept?: boolean }) {
  const { personName } = useConsole();
  if (!memory) return <div className="pa-muted">（这条记忆已经不存在）</div>;
  return (
    <div style={{ flex: 1, minWidth: 0, border: `1px solid ${kept ? "var(--pa-accent)" : "var(--pa-line)"}`, borderRadius: 12, padding: "10px 12px" }}>
      <div style={{ display: "flex", gap: 6, alignItems: "center", marginBottom: 4, flexWrap: "wrap" }}>
        <Pill tone="slate">{memory.typeLabel}</Pill>
        <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>{memory.subjectPerson ? personName(memory.subjectPerson) : "全家"} · 重要度 {memory.importance}</span>
        <span className="pa-num" style={{ fontSize: 12, color: "var(--pa-text-3)" }}>{dayjs(memory.occurredAt ?? memory.createdAt).format("YYYY-MM-DD")}</span>
      </div>
      <div style={{ fontWeight: 600, fontSize: 13.5 }}>{memory.title}</div>
      {memory.content !== memory.title && <div style={{ fontSize: 13, color: "var(--pa-text-2)", marginTop: 4, lineHeight: 1.6 }}>{memory.content}</div>}
      <div className="pa-mono" style={{ fontSize: 11.5, color: "var(--pa-text-3)", marginTop: 6 }}>{memory.memoryKey}</div>
    </div>
  );
}

function Merges() {
  const { message } = App.useApp();
  const { data, loading, reload } = useData(() => api.get<MemoryMergeSuggestion[]>("/insight/merges?status=pending"), []);
  const [finding, setFinding] = useState(false);
  const find = async () => {
    setFinding(true);
    try {
      const result = await api.post<{ added: number }>("/insight/merges/find");
      message.success(`新找到 ${result.added} 对`);
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setFinding(false);
    }
  };
  const decide = async (item: MemoryMergeSuggestion, keep: string) => {
    try {
      await api.post(`/insight/merges/${item.mergeKey}`, { keep });
      message.success(keep ? "已合并：另一条已归档" : "已忽略");
      void reload();
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  return (
    <>
      <div className="pa-toolbar">
        <span className="pa-muted" style={{ fontSize: 13 }}>同类型、同一个人、意思几乎一样的两条记忆。保留信息更全的一条，另一条会归档（不删除，可在记忆库找回）。</span>
        <div className="pa-toolbar-spacer" />
        <Button icon={<Copy size={15} />} className="pa-icon-text" loading={finding} onClick={find}>现在找一次</Button>
      </div>
      {loading && !data ? <div style={{ padding: 40, textAlign: "center" }}><Spin /></div> : !data?.length ? <Empty description="没有待处理的重复记忆" /> : (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {data.map((item) => (
            <div key={item.mergeKey} style={{ background: "var(--pa-bg)", borderRadius: 14, padding: 14 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
                <Pill tone="amber">相似度 {(item.similarity * 100).toFixed(1)}%</Pill>
                <span style={{ flex: 1 }} />
                {item.a && <Button size="small" onClick={() => decide(item, item.a!.memoryKey)}>保留左边</Button>}
                {item.b && <Button size="small" onClick={() => decide(item, item.b!.memoryKey)}>保留右边</Button>}
                <Button size="small" type="text" onClick={() => decide(item, "")}>不是重复</Button>
              </div>
              <div style={{ display: "flex", gap: 12 }}>
                <MemoryCard memory={item.a} />
                <MemoryCard memory={item.b} />
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function Runs() {
  const { data, loading } = useData(() => api.get<JobRun[]>("/insight/runs"), []);
  return (
    <Table<JobRun>
      rowKey={(run) => `${run.job}-${run.period}`}
      loading={loading}
      dataSource={data ?? []}
      pagination={false}
      locale={{ emptyText: "还没有跑过" }}
      columns={[
        { title: "时间", width: 150, render: (_, run) => <span className="pa-num">{dayjs(run.createdAt).format("MM-DD HH:mm")}</span> },
        { title: "任务", width: 100, render: (_, run) => jobLabel[run.job] ?? run.job },
        { title: "周期", width: 200, render: (_, run) => <span className="pa-mono" style={{ fontSize: 12 }}>{run.period}</span> },
        { title: "结果", render: (_, run) => <span style={{ fontSize: 13 }}>{run.result || "进行中"}</span> },
      ]}
    />
  );
}
