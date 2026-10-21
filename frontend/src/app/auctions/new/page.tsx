import { AuctionForm } from "@/components/AuctionForm";
import { RequireMember } from "@/lib/auth";

export default function NewAuctionPage() {
  return (
    <RequireMember>
      <AuctionForm />
    </RequireMember>
  );
}
