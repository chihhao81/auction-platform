import { Suspense } from "react";
import { RequireMember } from "@/lib/auth";
import { EditAuctionView } from "./EditAuctionView";

export default function EditAuctionPage() {
  return (
    <RequireMember>
      <Suspense fallback={null}>
        <EditAuctionView />
      </Suspense>
    </RequireMember>
  );
}
