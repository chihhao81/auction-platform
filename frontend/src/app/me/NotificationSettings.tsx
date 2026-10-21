"use client";

import { useEffect, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { disablePush, enablePush, getPushState, type PushState } from "@/lib/push";
import type { NotificationPreference } from "@/lib/types";

const STATE_TEXT: Record<PushState, string> = {
  unsupported: "此瀏覽器不支援推播通知（iPhone 需先「加入主畫面」）。",
  "server-disabled": "伺服器尚未啟用推播。",
  denied: "瀏覽器已封鎖通知，請在瀏覽器設定中允許此網站的通知。",
  enabled: "此裝置已開啟推播通知。",
  disabled: "此裝置尚未開啟推播通知。",
};

export function NotificationSettings() {
  const [pushState, setPushState] = useState<PushState | null>(null);
  const [prefs, setPrefs] = useState<NotificationPreference[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getPushState().then(setPushState, () => setPushState("unsupported"));
    api.get<{ items: NotificationPreference[] }>("/api/notifications/preferences").then((r) => setPrefs(r.items), () => {});
  }, []);

  async function toggleDevice() {
    setBusy(true);
    setError(null);
    try {
      if (pushState === "enabled") {
        await disablePush();
        setPushState("disabled");
      } else {
        setPushState(await enablePush());
      }
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "設定推播失敗");
    } finally {
      setBusy(false);
    }
  }

  async function togglePref(type: string, enabled: boolean) {
    setPrefs((p) => p.map((x) => (x.type === type ? { ...x, push_enabled: enabled } : x)));
    try {
      const res = await api.put<{ items: NotificationPreference[] }>("/api/notifications/preferences", { [type]: enabled });
      setPrefs(res.items);
    } catch {
      setPrefs((p) => p.map((x) => (x.type === type ? { ...x, push_enabled: !enabled } : x)));
    }
  }

  const canToggle = pushState === "enabled" || pushState === "disabled";

  return (
    <section id="notification-settings" className="space-y-3 rounded-xl bg-white p-5 shadow">
      <h2 className="text-lg font-bold">通知設定</h2>
      <div className="flex flex-wrap items-center gap-3">
        <p className="flex-1 text-sm text-gray-600">{pushState ? STATE_TEXT[pushState] : "檢查中…"}</p>
        {canToggle && (
          <button
            onClick={toggleDevice}
            disabled={busy}
            className="rounded border border-emerald-600 px-3 py-1.5 text-sm text-emerald-700 hover:bg-emerald-50 disabled:opacity-50"
          >
            {pushState === "enabled" ? "關閉此裝置推播" : "開啟此裝置推播"}
          </button>
        )}
      </div>
      {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
      <p className="text-xs text-gray-500">以下設定決定哪些通知會推播到你的裝置；站內通知列表一律保留。</p>
      <ul className="grid gap-2 sm:grid-cols-2">
        {prefs.map((p) => (
          <li key={p.type}>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={p.push_enabled} onChange={(e) => void togglePref(p.type, e.target.checked)} />
              {p.label}
            </label>
          </li>
        ))}
      </ul>
    </section>
  );
}
