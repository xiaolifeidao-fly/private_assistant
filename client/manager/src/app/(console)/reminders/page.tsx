"use client";

import dayjs from "dayjs";
import { Table, Tabs } from "antd";
import { api } from "@/lib/api";
import { useData } from "@/lib/hooks";
import type { Tone } from "@/lib/tone";
import type { Reminder, Trip } from "@/lib/types";
import { useConsole } from "@/components/console";
import { Kpi, PageHeader, Panel, Pill, Status } from "@/components/ui";

interface Delivery {
  deliveryKey: string;
  reminderKey: string;
  subscriptionKey: string;
  status: string;
  httpStatus: number;
  error: string;
  createdAt: string;
}

const kindMeta: Record<string, { label: string; tone: Tone }> = {
  manual: { label: "手工", tone: "slate" },
  birthday: { label: "生日", tone: "rose" },
  commitment: { label: "承诺", tone: "clay" },
  dated: { label: "带日期的事", tone: "plum" },
  festival: { label: "节日", tone: "amber" },
  trip: { label: "行程", tone: "sky" },
  daily: { label: "每日简报", tone: "sage" },
  weather: { label: "宝宝穿衣", tone: "sky" },
};
const statusMeta: Record<string, { label: string; tone: Tone; pulse?: boolean }> = {
  scheduled: { label: "待发送", tone: "sky" },
  sending: { label: "发送中", tone: "sky", pulse: true },
  sent: { label: "已发送", tone: "sage" },
  failed: { label: "失败", tone: "rose" },
  dismissed: { label: "已忽略", tone: "slate" },
  cancelled: { label: "已取消", tone: "slate" },
  planned: { label: "计划中", tone: "sky" },
  done: { label: "已完成", tone: "sage" },
};

function daysLater(value: string): string {
  const days = dayjs(value).startOf("day").diff(dayjs().startOf("day"), "day");
  if (days <= 0) return "今天";
  if (days === 1) return "明天";
  if (days === 2) return "后天";
  return `${days} 天后`;
}

function StatusOf({ value }: { value: string }) {
  const meta = statusMeta[value];
  return <Status tone={meta?.tone ?? "slate"} pulse={meta?.pulse}>{meta?.label ?? value}</Status>;
}

export default function RemindersPage() {
  const { placeName } = useConsole();
  const { data: reminders, loading } = useData(() => api.get<Reminder[]>("/reminders?days=180"), []);
  const { data: trips } = useData(() => api.get<Trip[]>("/trips"), []);
  const { data: deliveries } = useData(() => api.get<Delivery[]>("/push-deliveries"), []);

  const week = (reminders ?? []).filter((r) => dayjs(r.remindAt).diff(dayjs(), "day") < 7).length;
  const reminderTitle = (key: string) => reminders?.find((r) => r.reminderKey === key)?.title;
  const failed = (deliveries ?? []).filter((d) => d.status !== "sent").length;

  return (
    <>
      <PageHeader
        eyebrow="助理"
        title="提醒"
        description="生日、承诺到期、传统节日和回老家行程会自动派生提醒，通过 Web Push 推到手机。推送失败 5 次后停止重试；410/404 的过期订阅会自动停用。"
      />
      <div className="pa-kpis">
        <Kpi label="未来 180 天" value={reminders?.length ?? "—"} />
        <Kpi label="7 天内" value={week} tone="clay" />
        <Kpi label="计划中的行程" value={(trips ?? []).filter((t) => t.status === "planned").length} tone="sky" />
        <Kpi label="推送失败" value={failed} tone={failed ? "rose" : undefined} />
      </div>
      <Panel>
        <Tabs
          items={[
            {
              key: "reminders",
              label: "提醒",
              children: (
                <Table<Reminder>
                  rowKey="reminderKey"
                  scroll={{ x: 760 }}
                  loading={loading}
                  dataSource={reminders ?? []}
                  pagination={{ pageSize: 30, hideOnSinglePage: true, showSizeChanger: false }}
                  locale={{ emptyText: "未来 180 天没有提醒" }}
                  columns={[
                    {
                      title: "提醒时间",
                      width: 170,
                      render: (_, item) => (
                        <div>
                          <div className="pa-num" style={{ fontWeight: 550 }}>{dayjs(item.remindAt).format("M月D日 HH:mm")}</div>
                          <div style={{ fontSize: 12, color: "var(--pa-text-3)" }}>{dayjs(item.remindAt).format("YYYY")} · {daysLater(item.remindAt)}</div>
                        </div>
                      ),
                    },
                    { title: "类型", width: 120, render: (_, item) => <Pill tone={kindMeta[item.kind]?.tone}>{kindMeta[item.kind]?.label ?? item.kind}</Pill> },
                    {
                      title: "内容",
                      render: (_, item) => (
                        <div style={{ minWidth: 0 }}>
                          <div style={{ fontWeight: 550 }}>{item.title}</div>
                          {item.body && <div style={{ fontSize: 13, color: "var(--pa-text-3)", marginTop: 2, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{item.body}</div>}
                          {item.lastError && <div style={{ fontSize: 12.5, color: "var(--pa-danger)", marginTop: 2 }}>{item.lastError}</div>}
                        </div>
                      ),
                    },
                    { title: "状态", width: 110, render: (_, item) => <StatusOf value={item.status} /> },
                  ]}
                />
              ),
            },
            {
              key: "trips",
              label: "行程",
              children: (
                <Table<Trip>
                  rowKey="tripKey"
                  scroll={{ x: 760 }}
                  dataSource={trips ?? []}
                  pagination={false}
                  locale={{ emptyText: "还没有行程，在手机端「今天」里可以添加" }}
                  columns={[
                    { title: "出发", width: 170, render: (_, item) => <span className="pa-num">{dayjs(item.startAt).format("YYYY-MM-DD HH:mm")}</span> },
                    { title: "行程", render: (_, item) => <span style={{ fontWeight: 550 }}>{item.title}</span> },
                    { title: "目的地", width: 160, render: (_, item) => placeName(item.placeKey) },
                    { title: "备注", dataIndex: "notes", ellipsis: true },
                    { title: "状态", width: 110, render: (_, item) => <StatusOf value={item.status} /> },
                  ]}
                />
              ),
            },
            {
              key: "deliveries",
              label: "推送记录",
              children: (
                <Table<Delivery>
                  rowKey="deliveryKey"
                  scroll={{ x: 760 }}
                  dataSource={deliveries ?? []}
                  pagination={{ pageSize: 30, hideOnSinglePage: true, showSizeChanger: false }}
                  locale={{ emptyText: "还没有推送记录" }}
                  columns={[
                    { title: "时间", width: 170, render: (_, item) => <span className="pa-num">{dayjs(item.createdAt).format("YYYY-MM-DD HH:mm")}</span> },
                    { title: "提醒", dataIndex: "reminderKey", width: 240, render: (v) => reminderTitle(v) ?? <span className="pa-mono pa-muted">{v}</span> },
                    { title: "设备订阅", dataIndex: "subscriptionKey", width: 200, render: (v) => <span className="pa-mono pa-muted">{v}</span> },
                    { title: "结果", width: 130, render: (_, item) => <Status tone={item.status === "sent" ? "sage" : "rose"}>{item.status === "sent" ? "成功" : "失败"} · {item.httpStatus}</Status> },
                    { title: "错误", dataIndex: "error", ellipsis: true },
                  ]}
                />
              ),
            },
          ]}
        />
      </Panel>
    </>
  );
}
