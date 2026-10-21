import { Suspense } from "react";
import { RequireMember } from "@/lib/auth";
import { OrderDetailView } from "./OrderDetailView";

export default function OrderPage() {
  return (
    <RequireMember>
      <Suspense fallback={null}>
        <OrderDetailView />
      </Suspense>
    </RequireMember>
  );
}
