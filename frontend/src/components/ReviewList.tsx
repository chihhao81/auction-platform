import { formatTaipei } from "@/lib/time";
import type { UserReview } from "@/lib/types";

export function ReviewList({ reviews }: { reviews: UserReview[] }) {
  if (reviews.length === 0) return <p className="text-sm text-gray-500">尚無評價</p>;
  return (
    <ul className="divide-y">
      {reviews.map((r) => (
        <li key={r.id} className="py-2 text-sm">
          <div className="flex items-center gap-2">
            <span className={r.rating === "POSITIVE" ? "text-emerald-700" : "text-red-600"}>
              {r.rating === "POSITIVE" ? "👍 正評" : "👎 負評"}
            </span>
            <span className="text-gray-500">
              來自{r.from_buyer ? "買家" : "賣家"} {r.reviewer_name}
            </span>
            <span className="ml-auto text-xs text-gray-400">{formatTaipei(r.created_at)}</span>
          </div>
          {r.comment && <p className="mt-1 whitespace-pre-wrap break-words">{r.comment}</p>}
        </li>
      ))}
    </ul>
  );
}
