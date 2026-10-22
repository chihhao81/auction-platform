"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { STATUS_LABELS, formatPrice } from "@/lib/auction";
import { ORDER_STATUS_LABELS } from "@/lib/order";
import { REPORT_STATUS_LABELS } from "@/lib/reputation";
import { formatTaipei } from "@/lib/time";
import type { AuctionStatus, OrderStatus, ReportStatus } from "@/lib/types";
import { ReasonAction } from "./ReasonAction";

interface AdminReport {
  id: string;
  target_type: "USER" | "AUCTION";
  target_user_id: string | null;
  target_auction_id: string | null;
  order_id: string | null;
  reason_label: string;
  description: string;
  status: ReportStatus;
  resolution_note: string;
  reporter_name: string;
  target_user_name: string;
  target_auction_title: string;
  created_at: string;
}

interface AdminUser {
  id: string;
  display_name: string;
  role: string;
  status: string;
  created_at: string;
  active_ban_at: string | null;
  active_ban_reason: string | null;
}

interface BanRecord {
  id: string;
  reason: string;
  banned_at: string;
  banned_by_name: string;
  unbanned_at: string | null;
  unbanned_by_name: string;
  unban_reason: string;
}

interface AdminAuction {
  id: string;
  title: string;
  seller_id: string;
  seller_name: string;
  status: AuctionStatus;
  current_price: number;
  bid_count: number;
  end_time: string;
  settled: boolean;
  closed_at: string | null;
  close_reason: string;
}

interface AdminOrder {
  id: string;
  status: OrderStatus;
  buyer_name: string;
  seller_name: string;
  total_amount: number;
  overdue: boolean;
  created_at: string;
}

interface AttentionPayment {
  id: string;
  order_id: string;
  order_status: string;
  merchant_trade_no: string;
  provider_trade_no: string | null;
  amount: number;
  status: string;
  updated_at: string;
}

const TABS = [
  { key: "reports", label: "檢舉" },
  { key: "users", label: "使用者" },
  { key: "auctions", label: "競標" },
  { key: "orders", label: "訂單" },
  { key: "payments", label: "付款異常" },
  { key: "audit", label: "稽核紀錄" },
] as const;
type Tab = (typeof TABS)[number]["key"];

function useList<T>(url: string) {
  const [items, setItems] = useState<T[] | null>(null);
  const reload = useCallback(async () => {
    try {
      const r = await api.get<{ items: T[] }>(url);
      setItems(r.items);
    } catch {
      setItems([]);
    }
  }, [url]);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on url change
    void reload();
  }, [reload]);
  return { items, reload };
}

function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-xl bg-white shadow">
      <table className="w-full min-w-[640px] text-sm">
        <thead className="bg-gray-50 text-left text-gray-500">
          <tr>
            {head.map((h) => (
              <th key={h} className="px-3 py-2 font-normal">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y">{children}</tbody>
      </table>
    </div>
  );
}

function Loading({ items }: { items: unknown[] | null }) {
  if (items === null) return <p className="text-gray-500">載入中…</p>;
  if (items.length === 0) return <p className="text-gray-500">沒有資料</p>;
  return null;
}

function SearchBox({ onSearch, placeholder }: { onSearch: (q: string) => void; placeholder: string }) {
  const [q, setQ] = useState("");
  return (
    <form
      className="flex gap-2"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        onSearch(q.trim());
      }}
    >
      <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={placeholder} className="min-w-0 flex-1 rounded border px-3 py-1.5" />
      <button className="rounded bg-gray-700 px-3 text-white">搜尋</button>
    </form>
  );
}

function ReportsTab() {
  const [status, setStatus] = useState<string>("PENDING");
  const { items, reload } = useList<AdminReport>(`/api/admin/reports?status=${status}`);
  const update = (id: string, s: string) => (note: string) => api.post(`/api/admin/reports/${id}`, { status: s, note });
  return (
    <div className="space-y-3">
      <select value={status} onChange={(e) => setStatus(e.target.value)} className="rounded border px-2 py-1" aria-label="檢舉狀態">
        <option value="">全部</option>
        {Object.entries(REPORT_STATUS_LABELS).map(([k, v]) => (
          <option key={k} value={k}>{v}</option>
        ))}
      </select>
      <Loading items={items} />
      <ul className="space-y-2">
        {items?.map((r) => (
          <li key={r.id} className="space-y-1 rounded-xl bg-white p-3 text-sm shadow">
            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded bg-gray-100 px-1.5 text-xs">{REPORT_STATUS_LABELS[r.status]}</span>
              <span className="font-medium">{r.reason_label}</span>
              <span className="text-gray-500">檢舉人：{r.reporter_name}</span>
              <span className="ml-auto text-xs text-gray-400">{formatTaipei(r.created_at)}</span>
            </div>
            <p>
              對象：
              {r.target_auction_id ? (
                <Link className="underline" href={`/auctions/${r.target_auction_id}`}>競標「{r.target_auction_title}」</Link>
              ) : null}
              {r.target_user_id && (
                <Link className="ml-1 underline" href={`/users/${r.target_user_id}`}>使用者 {r.target_user_name}</Link>
              )}
              {r.order_id && (
                <span className="ml-1 text-gray-500">（訂單 {r.order_id.slice(0, 8)}）</span>
              )}
            </p>
            {r.description && <p className="whitespace-pre-wrap break-words text-gray-700">{r.description}</p>}
            {r.resolution_note && <p className="text-gray-500">處理說明：{r.resolution_note}</p>}
            {(r.status === "PENDING" || r.status === "REVIEWING") && (
              <div className="flex flex-wrap gap-2 pt-1">
                {r.status === "PENDING" && (
                  <ReasonAction label="標記處理中" placeholder="備註" onConfirm={update(r.id, "REVIEWING")} onDone={reload} />
                )}
                <ReasonAction label="已處理" placeholder="處理說明（會通知檢舉人）" onConfirm={update(r.id, "RESOLVED")} onDone={reload} />
                <ReasonAction label="不成立" danger placeholder="說明（會通知檢舉人）" onConfirm={update(r.id, "REJECTED")} onDone={reload} />
              </div>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function BanHistory({ userId }: { userId: string }) {
  const [bans, setBans] = useState<BanRecord[] | null>(null);
  useEffect(() => {
    api.get<{ bans: BanRecord[] }>(`/api/admin/users/${userId}`).then((r) => setBans(r.bans), () => setBans([]));
  }, [userId]);
  if (!bans) return <p className="text-xs text-gray-500">載入中…</p>;
  if (bans.length === 0) return <p className="text-xs text-gray-500">沒有停權紀錄</p>;
  return (
    <ul className="space-y-1 text-xs">
      {bans.map((b) => (
        <li key={b.id}>
          停權 {formatTaipei(b.banned_at)}（{b.banned_by_name}）：{b.reason}
          {b.unbanned_at && (
            <span className="text-gray-500">
              {" "}→ 解除 {formatTaipei(b.unbanned_at)}（{b.unbanned_by_name}）：{b.unban_reason}
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}

function UsersTab() {
  const [q, setQ] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const { items, reload } = useList<AdminUser>(`/api/admin/users?q=${encodeURIComponent(q)}`);
  return (
    <div className="space-y-3">
      <SearchBox onSearch={setQ} placeholder="搜尋名稱" />
      <Loading items={items} />
      {items && items.length > 0 && (
        <Table head={["名稱", "角色", "狀態", "加入時間", "操作"]}>
          {items.map((u) => (
            <tr key={u.id} className="align-top">
              <td className="px-3 py-2">
                <Link className="underline" href={`/users/${u.id}`}>{u.display_name}</Link>
                <button className="ml-2 text-xs text-gray-500 underline" onClick={() => setExpanded(expanded === u.id ? null : u.id)}>
                  停權紀錄
                </button>
                {expanded === u.id && <BanHistory userId={u.id} />}
              </td>
              <td className="px-3 py-2">{u.role === "ADMIN" ? "管理員" : "一般"}</td>
              <td className="px-3 py-2">
                {u.status === "BANNED" ? <span className="text-red-600">已停權</span> : "正常"}
                {u.active_ban_reason && <p className="text-xs text-gray-500">{u.active_ban_reason}</p>}
              </td>
              <td className="px-3 py-2">{formatTaipei(u.created_at)}</td>
              <td className="px-3 py-2">
                {u.role !== "ADMIN" &&
                  (u.status === "BANNED" ? (
                    <ReasonAction label="解除停權" onConfirm={(reason) => api.post(`/api/admin/users/${u.id}/unban`, { reason })} onDone={reload} />
                  ) : (
                    <ReasonAction label="停權" danger onConfirm={(reason) => api.post(`/api/admin/users/${u.id}/ban`, { reason })} onDone={reload} />
                  ))}
              </td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

function AuctionsTab() {
  const [q, setQ] = useState("");
  const { items, reload } = useList<AdminAuction>(`/api/admin/auctions?q=${encodeURIComponent(q)}`);
  return (
    <div className="space-y-3">
      <SearchBox onSearch={setQ} placeholder="搜尋商品名稱" />
      <Loading items={items} />
      {items && items.length > 0 && (
        <Table head={["商品", "賣家", "狀態", "目前價格", "結束", "操作"]}>
          {items.map((a) => (
            <tr key={a.id} className="align-top">
              <td className="px-3 py-2"><Link className="underline" href={`/auctions/${a.id}`}>{a.title}</Link></td>
              <td className="px-3 py-2"><Link className="underline" href={`/users/${a.seller_id}`}>{a.seller_name}</Link></td>
              <td className="px-3 py-2">
                {STATUS_LABELS[a.status]}
                {a.closed_at && <p className="text-xs text-red-600">管理員關閉：{a.close_reason}</p>}
              </td>
              <td className="px-3 py-2">{formatPrice(a.current_price)}（{a.bid_count}）</td>
              <td className="px-3 py-2">{formatTaipei(a.end_time)}</td>
              <td className="px-3 py-2">
                {!a.closed_at && !a.settled && (
                  <ReasonAction label="關閉競標" danger onConfirm={(reason) => api.post(`/api/admin/auctions/${a.id}/close`, { reason })} onDone={reload} />
                )}
              </td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

const CANCELLABLE: OrderStatus[] = ["AWAITING_CONTACT", "AWAITING_PAYMENT", "DISPUTED"];

function OrdersTab() {
  const [status, setStatus] = useState("");
  const { items, reload } = useList<AdminOrder>(`/api/admin/orders?status=${status}`);
  return (
    <div className="space-y-3">
      <select value={status} onChange={(e) => setStatus(e.target.value)} className="rounded border px-2 py-1" aria-label="訂單狀態">
        <option value="">全部</option>
        {Object.entries(ORDER_STATUS_LABELS).map(([k, v]) => (
          <option key={k} value={k}>{v}</option>
        ))}
      </select>
      <Loading items={items} />
      {items && items.length > 0 && (
        <Table head={["訂單", "買家", "賣家", "金額", "狀態", "操作"]}>
          {items.map((o) => (
            <tr key={o.id} className="align-top">
              <td className="px-3 py-2 font-mono text-xs">{o.id.slice(0, 8)}<p className="font-sans text-gray-400">{formatTaipei(o.created_at)}</p></td>
              <td className="px-3 py-2">{o.buyer_name}</td>
              <td className="px-3 py-2">{o.seller_name}</td>
              <td className="px-3 py-2">{formatPrice(o.total_amount)}</td>
              <td className="px-3 py-2">
                {ORDER_STATUS_LABELS[o.status]}
                {o.overdue && <span className="ml-1 text-red-600">逾期</span>}
              </td>
              <td className="px-3 py-2">
                {CANCELLABLE.includes(o.status) && (
                  <ReasonAction label="取消訂單" danger onConfirm={(reason) => api.post(`/api/admin/orders/${o.id}/cancel`, { reason })} onDone={reload} />
                )}
              </td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

function PaymentsTab() {
  const { items } = useList<AttentionPayment>("/api/admin/payments/attention");
  return (
    <div className="space-y-3">
      <p className="text-sm text-gray-600">
        綠界已確認收款、但未套用到訂單的付款（重複付款或金額不符），需至綠界商店後台人工退款或核對。
      </p>
      <Loading items={items} />
      {items && items.length > 0 && (
        <Table head={["綠界單號", "訂單", "金額", "狀態", "時間"]}>
          {items.map((p) => (
            <tr key={p.id}>
              <td className="px-3 py-2 font-mono text-xs">{p.merchant_trade_no}<p>{p.provider_trade_no}</p></td>
              <td className="px-3 py-2 font-mono text-xs">{p.order_id.slice(0, 8)}（{p.order_status}）</td>
              <td className="px-3 py-2">{formatPrice(p.amount)}</td>
              <td className="px-3 py-2 text-red-600">{p.status === "DUPLICATE" ? "重複付款" : "金額不符"}</td>
              <td className="px-3 py-2">{formatTaipei(p.updated_at)}</td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

interface AuditEntry {
  id: string;
  actor_id: string | null;
  actor_name: string;
  action: string;
  target_type: string;
  target_id: string;
  metadata: Record<string, unknown>;
  ip: string;
  created_at: string;
}

function AuditTab() {
  const [filter, setFilter] = useState({ action: "", target_id: "" });
  const [applied, setApplied] = useState(filter);
  const qs = new URLSearchParams(applied).toString();
  const { items } = useList<AuditEntry>(`/api/admin/audit-logs?${qs}`);
  return (
    <div className="space-y-3">
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          setApplied({ action: filter.action.trim(), target_id: filter.target_id.trim() });
        }}
      >
        <input value={filter.action} onChange={(e) => setFilter({ ...filter, action: e.target.value })}
          placeholder="動作（如 BID_PLACE、USER_BAN）" className="rounded border px-3 py-1.5" aria-label="動作" />
        <input value={filter.target_id} onChange={(e) => setFilter({ ...filter, target_id: e.target.value })}
          placeholder="對象 ID" className="min-w-0 flex-1 rounded border px-3 py-1.5" aria-label="對象 ID" />
        <button className="rounded bg-gray-700 px-3 text-white">查詢</button>
      </form>
      <p className="text-xs text-gray-500">稽核紀錄只能新增，任何人（含管理員）都無法修改或刪除。</p>
      <Loading items={items} />
      {items && items.length > 0 && (
        <Table head={["時間", "操作者", "動作", "對象", "內容", "IP"]}>
          {items.map((a) => (
            <tr key={a.id} className="align-top">
              <td className="whitespace-nowrap px-3 py-2">{formatTaipei(a.created_at)}</td>
              <td className="px-3 py-2">{a.actor_name || "（系統）"}</td>
              <td className="px-3 py-2 font-mono text-xs">{a.action}</td>
              <td className="px-3 py-2 font-mono text-xs">{a.target_type}<br />{a.target_id.slice(0, 8)}</td>
              <td className="max-w-xs break-all px-3 py-2 font-mono text-xs">{JSON.stringify(a.metadata)}</td>
              <td className="px-3 py-2 text-xs">{a.ip}</td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

export function AdminConsole() {
  const { state } = useAuth();
  const [tab, setTab] = useState<Tab>("reports");
  // UX guard only: every /api/admin route also requires the ADMIN role.
  if (state.status !== "authenticated" || state.me.user.role !== "ADMIN") {
    return <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">需要管理員權限</p>;
  }
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-bold">管理後台</h1>
      <div className="flex flex-wrap gap-2" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.key}
            role="tab"
            aria-selected={tab === t.key}
            onClick={() => setTab(t.key)}
            className={`rounded-full px-3 py-1 text-sm ${tab === t.key ? "bg-amber-600 text-white" : "bg-white shadow-sm"}`}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "reports" && <ReportsTab />}
      {tab === "users" && <UsersTab />}
      {tab === "auctions" && <AuctionsTab />}
      {tab === "orders" && <OrdersTab />}
      {tab === "payments" && <PaymentsTab />}
      {tab === "audit" && <AuditTab />}
    </div>
  );
}
