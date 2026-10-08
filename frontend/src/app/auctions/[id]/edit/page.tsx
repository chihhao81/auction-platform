import { AuctionCreateForm } from "@/components/auction-create-form";

export default async function EditAuctionPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <AuctionCreateForm auctionId={id} />;
}
