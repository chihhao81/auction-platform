import { STATUS_LABELS } from "@/lib/auction";
import type { AuctionStatus } from "@/lib/types";

const COLORS: Record<AuctionStatus, string> = {
  UPCOMING: "bg-sky-100 text-sky-800",
  ACTIVE: "bg-emerald-100 text-emerald-800",
  ENDED: "bg-gray-200 text-gray-700",
  CLOSED: "bg-gray-200 text-gray-500",
};

export function StatusBadge({ status }: { status: AuctionStatus }) {
  return <span className={`rounded px-2 py-0.5 text-xs font-medium ${COLORS[status]}`}>{STATUS_LABELS[status]}</span>;
}
