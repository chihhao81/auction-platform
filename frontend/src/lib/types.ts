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
