import { Suspense } from "react";
import { AuctionDetailView } from "./AuctionDetailView";

export default function AuctionPage() {
  return (
    <Suspense fallback={<p className="text-center text-gray-500">載入中…</p>}>
      <AuctionDetailView />
    </Suspense>
  );
}
