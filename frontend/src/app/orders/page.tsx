import { RequireMember } from "@/lib/auth";
import { OrderList } from "./OrderList";

export default function OrdersPage() {
  return (
    <RequireMember>
      <OrderList />
    </RequireMember>
  );
}
