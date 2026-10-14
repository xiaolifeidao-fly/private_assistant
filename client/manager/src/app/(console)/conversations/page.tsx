"use client";

import { useState } from "react";
import dayjs from "dayjs";
import { App, Button, Collapse, Drawer, Empty, Segmented, Spin, Table } from "antd";
import { ClipboardCopy, FileCode2, ShieldAlert, ThumbsDown, ThumbsUp, TriangleAlert, Wrench } from "lucide-react";
import { api, query } from "@/lib/api";
import { useData } from "@/lib/hooks";
import { toolLabel } from "@/lib/labels";
import { botTone } from "@/lib/tone";
import type { Bot, ChatMessage, Conversation, Paged } from "@/lib/types";
import { RichText } from "@/components/rich-text";
import { useConsole } from "@/components/console";
import { Avatar, Callout, PageHeader, Panel, Pill } from "@/components/ui";

export default function ConversationsPage() {
  const [botKey, setBotKey] = useState("");
  const [page, setPage] = useState(1);
  const [viewing, setViewing] = useState<Conversation | null>(null);
  const { data: bots } = useData(() => api.get<Bot[]>("/bots"), []);
  const { data, loading } = useData(() => api.get<Paged<Conversation>>(`/conversations${query({ botKey, page, size: 30 })}`), [botKey, page]);
  const botOf = (key: string) => bots?.find((b) => b.botKey === key);

  return (
    <>
      <PageHeader
        eyebrow="助理"
        title="对话记录"
        description="手机端所有对话的完整记录：每轮自动检索到的参考、调用了哪些工具、用了多少 token，以及你给的反馈。"
      />
      <Panel>
        <div className="pa-toolbar">
          <Segmented
            value={botKey}
            onChange={(value) => { setBotKey(String(value)); setPage(1); }}
            options={[{ value: "", label: "全部" }, ...(bots ?? []).map((b) => ({ value: b.botKey, label: b.name }))]}
          />
          <div className="pa-toolbar-spacer" />
          <span className="pa-muted pa-num" style={{ fontSize: 13 }}>共 {data?.total ?? 0} 段</span>
        </div>
        <Table<Conversation>
          rowKey="conversationKey"
          scroll={{ x: 640 }}
          loading={loading}
          dataSource={data?.items ?? []}
          onRow={(c) => ({ onClick: () => setViewing(c), style: { cursor: "pointer" } })}
          pagination={{ current: page, pageSize: 30, total: data?.total ?? 0, onChange: setPage, hideOnSinglePage: true, showSizeChanger: false }}
          locale={{ emptyText: "还没有对话" }}
          columns={[
            {
              title: "对话",
              render: (_, c) => {
                const bot = botOf(c.botKey);
                return (
                  <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                    <Avatar tone={botTone(c.botKey)} size={34}>{bot?.avatar ?? "💬"}</Avatar>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontWeight: 550 }}>{c.title}</div>
                      <div style={{ fontSize: 12.5, color: "var(--pa-text-3)", marginTop: 1 }}>{bot?.name ?? c.botKey}</div>
                    </div>
                  </div>
                );
              },
            },
            { title: "消息", dataIndex: "messageCount", width: 90, render: (v) => <span className="pa-num">{v}</span> },
            { title: "最近", width: 170, render: (_, c) => <span className="pa-num" style={{ color: "var(--pa-text-2)" }}>{dayjs(c.lastMessageAt ?? c.createdAt).format("YYYY-MM-DD HH:mm")}</span> },
          ]}
        />
      </Panel>
      {viewing && <ConversationViewer conversation={viewing} bot={botOf(viewing.botKey)} onClose={() => setViewing(null)} />}
    </>
  );
}

function ConversationViewer({ conversation, bot, onClose }: { conversation: Conversation; bot?: Bot; onClose: () => void }) {
  const { data: messages, loading } = useData(() => api.get<ChatMessage[]>(`/conversations/${conversation.conversationKey}/messages`), [conversation.conversationKey]);
  const [showSystem, setShowSystem] = useState(false);
  // 系统提示词很长，点开才拉
  const { data: system, loading: systemLoading } = useData(
    () => (showSystem ? api.get<{ system: string }>(`/conversations/${conversation.conversationKey}/system`) : Promise.resolve(undefined)),
    [conversation.conversationKey, showSystem],
  );

  return (
    <Drawer
      title={
        <span style={{ display: "inline-flex", alignItems: "center", gap: 10, minWidth: 0 }}>
          <Avatar tone={botTone(conversation.botKey)} size={28}>{bot?.avatar ?? "💬"}</Avatar>
          <span style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{conversation.title}</span>
        </span>
      }
      width={800}
      open
      onClose={onClose}
      extra={<Button icon={<FileCode2 size={15} />} className="pa-icon-text" onClick={() => setShowSystem(true)}>系统提示词</Button>}
      styles={{ body: { background: "var(--pa-bg)" } }}
    >
      {loading && !messages ? <div style={{ textAlign: "center", padding: 40 }}><Spin /></div> : !messages?.length ? <Empty description="这段对话还没有消息" /> : (
        <div style={{ display: "flex", flexDirection: "column", gap: 14, maxWidth: 680, margin: "0 auto" }}>
          {messages.map((message, index) => (
            <div key={message.messageKey} style={{ display: "contents" }}>
              {index > 0 && message.segment !== messages[index - 1].segment && (
                <div style={{ display: "flex", alignItems: "center", gap: 10, color: "var(--pa-text-3)", fontSize: 12.5 }}>
                  <span style={{ flex: 1, borderTop: "1px dashed var(--pa-line-strong, var(--pa-line))" }} />
                  第 {message.segment + 1} 段：前面的内容已压缩成摘要，模型只看得到摘要
                  <span style={{ flex: 1, borderTop: "1px dashed var(--pa-line-strong, var(--pa-line))" }} />
                </div>
              )}
              <MessageItem message={message} bot={bot} />
            </div>
          ))}
        </div>
      )}
      <Drawer title="会话冻结的系统提示词" width={720} open={showSystem} onClose={() => setShowSystem(false)}>
        {systemLoading && !system ? <div style={{ textAlign: "center", padding: 40 }}><Spin /></div> : (
          <pre className="pa-code" style={{ maxHeight: "none" }}>{system?.system || "（会话还没有发过消息）"}</pre>
        )}
      </Drawer>
    </Drawer>
  );
}

function MessageItem({ message, bot }: { message: ChatMessage; bot?: Bot }) {
  const { personName } = useConsole();
  const time = dayjs(message.createdAt).format("MM-DD HH:mm:ss");
  if (message.role === "notice") {
    return (
      <Callout tone="amber" icon={<ShieldAlert size={16} />}>
        <div style={{ fontWeight: 600, marginBottom: 4 }}>系统补充的求助提示（回答里漏了）</div>
        <div className="pa-pre" style={{ fontSize: 13 }}>{message.content}</div>
      </Callout>
    );
  }
  if (message.role === "user") {
    const context = message.blocks?.find((b) => b.text?.startsWith("<turn_context>"));
    const summary = message.blocks?.find((b) => b.text?.startsWith("<previous_summary>"));
    const meta = message.meta;
    return (
      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 6 }}>
        <div className="pa-pre" style={{ maxWidth: "82%", padding: "10px 15px", borderRadius: "18px 18px 6px 18px", background: "var(--pa-text)", color: "var(--pa-bg)", fontSize: 14.5, lineHeight: 1.65 }}>
          {message.content}
        </div>
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", justifyContent: "flex-end", alignItems: "center" }}>
          {meta?.safety?.signals?.length ? <Pill tone="rose"><ShieldAlert size={11} />危机信号：{meta.safety.signals.join("、")}</Pill> : null}
          {meta?.segment?.rolled && (
            <Pill tone="amber">
              已压缩前文，开始第 {meta.segment.to + 1} 段{meta.segment.lost?.length ? ` · 摘要漏了 ${meta.segment.lost.length} 条记忆` : ""}
            </Pill>
          )}
          {meta?.personaUpdates?.length ? (
            <span title={meta.personaUpdates.map((u) => `${personName(u.person)}：${u.changes}`).join("\n")}>
              <Pill tone="sage">人设卡更新：{meta.personaUpdates.map((u) => `${personName(u.person)} v${u.to}`).join("、")}</Pill>
            </span>
          ) : null}
          {meta?.retrieval && (
            meta.retrieval.skipped
              ? <Pill tone="slate">寒暄，跳过检索</Pill>
              : <Pill tone="slate">检索 {meta.retrieval.hits} 条{meta.retrieval.deduped ? ` · 去重 ${meta.retrieval.deduped}` : ""}{meta.retrieval.clipped ? ` · 超预算 ${meta.retrieval.clipped}` : ""}</Pill>
          )}
          <span className="pa-num" style={{ fontSize: 11.5, color: "var(--pa-text-3)" }}>{time}</span>
        </div>
        {(context || summary) && (
          <Collapse
            size="small"
            ghost
            style={{ width: "100%" }}
            items={[
              ...(summary ? [{ key: "summary", label: <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>上一段的压缩摘要</span>, children: <pre className="pa-code">{summary.text}</pre> }] : []),
              ...(context ? [{ key: "ctx", label: <span style={{ fontSize: 12.5, color: "var(--pa-text-3)" }}>本轮自动检索的参考</span>, children: <pre className="pa-code">{context.text}</pre> }] : []),
            ]}
          />
        )}
      </div>
    );
  }
  if (message.role === "tool") {
    return (
      <Collapse
        size="small"
        style={{ background: "var(--pa-surface)" }}
        items={[{
          key: "tool",
          label: (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--pa-text-2)" }}>
              <Wrench size={13} />
              {(message.blocks ?? []).map((b) => toolLabel[b.toolName ?? ""] ?? b.toolName).join("、")}
            </span>
          ),
          children: (message.blocks ?? []).map((block) => (
            <div key={block.toolUseId} style={{ marginBottom: 10 }}>
              <div style={{ marginBottom: 6 }}><Pill tone={block.isError ? "rose" : "slate"}>{block.toolName}</Pill></div>
              <pre className="pa-code">{block.text}</pre>
            </div>
          )),
        }]}
      />
    );
  }
  const toolUses = (message.blocks ?? []).filter((b) => b.type === "tool_use");
  return (
    <div style={{ background: "var(--pa-surface)", border: "1px solid var(--pa-line)", borderRadius: 14, padding: "14px 18px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10, flexWrap: "wrap" }}>
        <Avatar tone={botTone(bot?.botKey ?? "")} size={22}>{bot?.avatar ?? "🤖"}</Avatar>
        <span style={{ fontWeight: 600, fontSize: 13 }}>{bot?.name ?? "助理"}</span>
        <span className="pa-num" style={{ fontSize: 12, color: "var(--pa-text-3)" }}>{time}</span>
        {message.model && <span className="pa-mono" style={{ fontSize: 11.5, color: "var(--pa-text-3)" }}>{message.model}</span>}
        <span style={{ flex: 1 }} />
        {message.feedback === "up" && <Pill tone="sage"><ThumbsUp size={11} />有用</Pill>}
        {message.feedback === "down" && <Pill tone="rose"><ThumbsDown size={11} />不对</Pill>}
        {message.meta?.citations?.unknown?.length ? <Pill tone="rose"><TriangleAlert size={11} />{message.meta.citations.unknown.length} 条引用无出处</Pill> : null}
        {message.status === "error" && <Pill tone="rose"><TriangleAlert size={11} />失败</Pill>}
      </div>
      {message.status === "error" ? <div style={{ color: "var(--pa-danger)" }}>{message.error}</div> : message.content && <RichText text={message.content} />}
      {toolUses.length > 0 && (
        <div style={{ marginTop: message.content ? 12 : 0, display: "flex", flexDirection: "column", gap: 4 }}>
          {toolUses.map((use) => (
            <div key={use.toolUseId} className="pa-mono" style={{ color: "var(--pa-text-3)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
              → {use.toolName} {JSON.stringify(use.input)}
            </div>
          ))}
        </div>
      )}
      {message.meta?.citations?.unknown?.length ? (
        <div className="pa-mono" style={{ marginTop: 10, fontSize: 12, color: "var(--pa-danger)" }}>
          模型在这段对话里没见过这些编号：{message.meta.citations.unknown.join("、")}
        </div>
      ) : null}
      {message.feedbackNote && (
        <div style={{ marginTop: 12, padding: "8px 12px", borderRadius: 10, background: "var(--tone-rose-bg)", fontSize: 13 }}>反馈：{message.feedbackNote}</div>
      )}
      {message.feedback === "down" && <ExportEvalCase messageKey={message.messageKey} />}
      {message.usage && (
        <div className="pa-num" style={{ fontSize: 11.5, color: "var(--pa-text-3)", marginTop: 12, display: "flex", gap: 14 }}>
          <span>输入 {message.usage.inputTokens}</span>
          <span>缓存读 {message.usage.cacheReadTokens}</span>
          <span>输出 {message.usage.outputTokens}</span>
        </div>
      )}
    </div>
  );
}

/** 把点踩的回答转成评测用例 JSON，复制到剪贴板。里面是真实对话，进仓库前要改写成虚构内容。 */
function ExportEvalCase({ messageKey }: { messageKey: string }) {
  const { message, modal } = App.useApp();
  const [loading, setLoading] = useState(false);
  const exportCase = async () => {
    setLoading(true);
    try {
      const result = await api.get<Record<string, unknown>>(`/messages/${messageKey}/eval-case`);
      const text = JSON.stringify(result, null, 2);
      try {
        await navigator.clipboard.writeText(text);
        message.success("已复制。贴进 server/eval/cases/ 前，先把真实人名和原话改成虚构内容");
      } catch {
        modal.info({ title: "评测用例", width: 640, content: <pre className="pa-code">{text}</pre> });
      }
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setLoading(false);
    }
  };
  return (
    <Button size="small" style={{ marginTop: 10 }} icon={<ClipboardCopy size={13} />} className="pa-icon-text" loading={loading} onClick={exportCase}>
      转成评测用例
    </Button>
  );
}
