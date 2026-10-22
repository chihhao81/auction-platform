// DTOs returned by the backend API.

export interface Me {
  user: {
    id: string;
    display_name: string;
    avatar_url: string;
    role: "USER" | "ADMIN";
    status: "ACTIVE" | "BANNED";
    created_at: string;
  };
  terms: {
    current_version: string;
    accepted: boolean;
  };
}

export interface Terms {
  version: string;
  content: string;
}

export type AuctionStatus = "UPCOMING" | "ACTIVE" | "ENDED" | "CLOSED";

export interface AuctionImage {
  id: string;
  url: string;
}

export interface AuctionSummary {
  id: string;
  title: string;
  images: AuctionImage[];
  current_price: number;
  bid_count: number;
  start_time: string;
  end_time: string;
  status: AuctionStatus;
  seller_name: string;
}

export interface AuctionList {
  items: AuctionSummary[];
  server_time: string;
}

export interface Bid {
  id: string;
  amount: number;
  bidder_name: string;
  is_mine: boolean;
  created_at: string;
}

export interface AuctionDetail extends AuctionSummary {
  description: string;
  start_price: number;
  min_increment: number;
  max_increment: number;
  original_end_time: string;
  shipping_method: string;
  shipping_fee: number;
  seller: { id: string; display_name: string; avatar_url: string };
  bids: Bid[];
  next_bid_min: number;
  next_bid_max: number;
  is_seller: boolean;
  is_highest_bidder: boolean;
  contact_info?: string;
}

export interface AuctionDetailResponse {
  auction: AuctionDetail;
  server_time: string;
}

export interface AuctionPrice {
  id: string;
  current_price: number;
  bid_count: number;
  end_time: string;
  status: AuctionStatus;
}

export interface PriceList {
  items: AuctionPrice[];
  server_time: string;
}

export interface BidResponse {
  bid_id: string;
  current_price: number;
  bid_count: number;
  end_time: string;
  extended: boolean;
  next_bid_min: number;
  next_bid_max: number;
}

export interface AppNotification {
  id: string;
  type: string;
  title: string;
  content: string;
  related_auction_id: string | null;
  related_order_id: string | null;
  link: string;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPreference {
  type: string;
  label: string;
  push_enabled: boolean;
}

export type OrderStatus =
  | "AWAITING_CONTACT"
  | "AWAITING_PAYMENT"
  | "PAID"
  | "AWAITING_SHIPMENT"
  | "SHIPPED"
  | "RECEIVED"
  | "COMPLETED"
  | "CANCELLED"
  | "DISPUTED";

export interface OrderSummary {
  id: string;
  status: OrderStatus;
  total_amount: number;
  item_count: number;
  counterparty_name: string;
  overdue: boolean;
  created_at: string;
}

export interface OrderParty {
  id: string;
  display_name: string;
  avatar_url: string;
}

export interface OrderDetail {
  id: string;
  status: OrderStatus;
  role: "buyer" | "seller";
  buyer: OrderParty;
  seller: OrderParty;
  items: {
    auction_id: string;
    title: string;
    amount: number;
    shipping_fee: number;
    shipping_method: string;
    image_url: string;
  }[];
  items_total: number;
  shipping_fee: number;
  total_amount: number;
  contact_deadline: string;
  contacted_at: string | null;
  overdue: boolean;
  seller_contact: string[];
  paid_at: string | null;
  shipped_at: string | null;
  received_at: string | null;
  completed_at: string | null;
  shipment: Shipment | null;
  my_review: OrderReview | null;
  their_review: OrderReview | null;
  auto_advance_at: string | null;
  created_at: string;
}

export interface Shipment {
  recipient_name: string;
  recipient_phone: string;
  recipient_address: string;
  carrier: string;
  tracking_number: string;
  shipped_at: string | null;
  purged: boolean;
}

export interface OrderMessage {
  id: string;
  mine: boolean;
  content: string;
  created_at: string;
  read_at: string | null;
}

export interface PaymentCheckout {
  action: string;
  fields: Record<string, string>;
}

export interface Reputation {
  positive: number;
  negative: number;
  completed_orders: number;
}

export interface UserReview {
  id: string;
  rating: "POSITIVE" | "NEGATIVE";
  comment: string;
  reviewer_name: string;
  from_buyer: boolean;
  created_at: string;
}

export interface PublicProfile {
  id: string;
  display_name: string;
  avatar_url: string;
  banned: boolean;
  created_at: string;
  reputation: Reputation;
  reviews: UserReview[];
}

export interface OrderReview {
  rating: "POSITIVE" | "NEGATIVE";
  comment: string;
  created_at: string;
}

export type ReportStatus = "PENDING" | "REVIEWING" | "RESOLVED" | "REJECTED";

export interface Report {
  id: string;
  target_type: "USER" | "AUCTION";
  target_user_id: string | null;
  target_auction_id: string | null;
  reason: string;
  reason_label: string;
  description: string;
  status: ReportStatus;
  resolution_note: string;
  handled_at: string | null;
  created_at: string;
}

export type ReportReasons = Record<"USER" | "AUCTION", Record<string, string>>;
