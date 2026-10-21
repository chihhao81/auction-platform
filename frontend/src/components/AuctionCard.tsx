import Link from "next/link";
import { formatPrice } from "@/lib/auction";
import { formatRemaining, formatTaipei } from "@/lib/time";
import type { AuctionSummary } from "@/lib/types";
import { StatusBadge } from "./StatusBadge";

export function AuctionCard({ auction, now }: { auction: AuctionSummary; now: number }) {
  const cover = auction.images[0];
  const upcoming = auction.status === "UPCOMING";
  const remaining = new Date(upcoming ? auction.start_time : auction.end_time).getTime() - now;
  return (
    <Link
      href={`/auctions/${auction.id}`}
      className="flex flex-col overflow-hidden rounded-xl bg-white shadow transition hover:shadow-md"
    >
      <div className="relative aspect-square bg-gray-100">
        {cover ? (
          // eslint-disable-next-line @next/next/no-img-element -- images are already resized by the backend
          <img src={cover.url} alt={auction.title} loading="lazy" className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full items-center justify-center text-4xl">🦎</div>
        )}
        <div className="absolute left-2 top-2">
          <StatusBadge status={auction.status} />
        </div>
      </div>
      <div className="flex flex-1 flex-col gap-1 p-3">
        <h2 className="line-clamp-2 text-sm font-medium">{auction.title}</h2>
        <p className="text-lg font-bold text-emerald-700" data-testid="price">
          {formatPrice(auction.current_price)}
        </p>
        <p className="mt-auto text-xs text-gray-500">
          {upcoming ? "開始" : "結束"} {formatTaipei(upcoming ? auction.start_time : auction.end_time)}
          {auction.status !== "ENDED" && remaining > 0 && <span className="ml-1">（{formatRemaining(remaining)}）</span>}
        </p>
      </div>
    </Link>
  );
}
