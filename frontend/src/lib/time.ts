// All times from the backend are UTC ISO strings. They are always displayed
// in Asia/Taipei regardless of the viewer's local timezone.

export const DISPLAY_TIMEZONE = "Asia/Taipei";

const dateTimeFormat = new Intl.DateTimeFormat("zh-TW", {
  timeZone: DISPLAY_TIMEZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});

export function formatTaipei(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  if (Number.isNaN(d.getTime())) return "";
  const parts = Object.fromEntries(dateTimeFormat.formatToParts(d).map((p) => [p.type, p.value]));
  return `${parts.year}/${parts.month}/${parts.day} ${parts.hour}:${parts.minute}`;
}

// Converts a value from <input type="datetime-local"> (interpreted as Taipei
// time, UTC+8 with no DST) into a UTC ISO string for the backend.
export function taipeiLocalInputToISO(value: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (!m) throw new Error("invalid datetime-local value");
  const [, y, mo, d, h, mi] = m.map(Number);
  return new Date(Date.UTC(y, mo - 1, d, h - 8, mi)).toISOString();
}

// Inverse of taipeiLocalInputToISO, for pre-filling datetime-local inputs.
export function isoToTaipeiLocalInput(iso: string): string {
  const d = new Date(new Date(iso).getTime() + 8 * 3600_000);
  return d.toISOString().slice(0, 16);
}

// Human readable remaining time. `now` must come from the server clock
// offset (see useServerClock) — never trust the browser clock for business
// decisions; this is display only.
export function formatRemaining(ms: number): string {
  if (ms <= 0) return "已結束";
  const s = Math.floor(ms / 1000);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (d > 0) return `${d} 天 ${h} 小時`;
  if (h > 0) return `${h} 小時 ${m} 分`;
  if (m > 0) return `${m} 分 ${sec} 秒`;
  return `${sec} 秒`;
}
