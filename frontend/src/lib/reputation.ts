import type { ReportStatus, Reputation } from "./types";

// "98% 正評（49/50）" style summary; null when there are no reviews yet.
export function reputationSummary(r: Reputation): string | null {
  const total = r.positive + r.negative;
  if (total === 0) return null;
  const pct = Math.round((r.positive / total) * 100);
  return `${pct}% 正評（${r.positive}/${total}）`;
}

export const REPORT_STATUS_LABELS: Record<ReportStatus, string> = {
  PENDING: "待處理",
  REVIEWING: "處理中",
  RESOLVED: "已處理",
  REJECTED: "不成立",
};
