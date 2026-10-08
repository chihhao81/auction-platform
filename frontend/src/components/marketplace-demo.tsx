"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

type Category = "全部商品" | "守宮" | "鼠婦" | "其他活體";
type Auction = {
  id: number;
  title: string;
  category: Exclude<Category, "全部商品">;
  species: string;
  currentPrice: number;
  bids: number;
  remaining: string;
  seller: string;
  badge: string;
  icon: string;
  theme: string;
  status: "進行中" | "即將開始";
};

const auctions: Auction[] = [
  { id: 1, title: "橘夢豹紋守宮", category: "守宮", species: "豹紋守宮・母", currentPrice: 2800, bids: 8, remaining: "05 小時 24 分", seller: "島上飼育家", badge: "熱門競標", icon: "🦎", theme: "geckoSunset", status: "進行中" },
  { id: 2, title: "奶油白暴龍守宮", category: "守宮", species: "睫角守宮・幼體", currentPrice: 1600, bids: 5, remaining: "11 小時 08 分", seller: "森林邊界", badge: "剛上架", icon: "🦎", theme: "geckoMint", status: "進行中" },
  { id: 3, title: "白化鼠婦繁殖組", category: "鼠婦", species: "鼠婦・成體 6 隻", currentPrice: 450, bids: 3, remaining: "明日 20:00 開始", seller: "小小甲蟲屋", badge: "即將開始", icon: "🐚", theme: "isopodLilac", status: "即將開始" },
  { id: 4, title: "橙斑豹紋守宮", category: "守宮", species: "豹紋守宮・公", currentPrice: 3200, bids: 12, remaining: "02 小時 16 分", seller: "島上飼育家", badge: "即將結標", icon: "🦎", theme: "geckoApricot", status: "進行中" },
  { id: 5, title: "藍色夢幻鼠婦", category: "鼠婦", species: "鼠婦・成體 4 隻", currentPrice: 680, bids: 6, remaining: "18 小時 42 分", seller: "土壤研究室", badge: "人氣上升", icon: "🐚", theme: "isopodBlue", status: "進行中" },
  { id: 6, title: "角蛙照護入門組", category: "其他活體", species: "角蛙・附飼養箱", currentPrice: 900, bids: 2, remaining: "週五 19:30 開始", seller: "綠意日常", badge: "即將開始", icon: "🐸", theme: "frogGreen", status: "即將開始" },
];

const categories: Category[] = ["全部商品", "守宮", "鼠婦", "其他活體"];
const money = new Intl.NumberFormat("zh-TW");

export function MarketplaceDemo() {
  const [category, setCategory] = useState<Category>("全部商品");
  const [status, setStatus] = useState<"全部" | Auction["status"]>("全部");
  const [query, setQuery] = useState("");
  const [favorites, setFavorites] = useState<number[]>([2]);
  const [notice, setNotice] = useState("");

  const filteredAuctions = useMemo(() => auctions.filter((auction) => {
    const matchesCategory = category === "全部商品" || auction.category === category;
    const matchesStatus = status === "全部" || auction.status === status;
    const searchable = `${auction.title} ${auction.species} ${auction.seller}`.toLocaleLowerCase();
    return matchesCategory && matchesStatus && searchable.includes(query.trim().toLocaleLowerCase());
  }), [category, query, status]);

  function toggleFavorite(id: number) {
    setFavorites((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }

  return (
    <main className="marketShell">
      <div className="demoBanner"><span className="demoPulse" />展示模式・登入、商品與價格皆為模擬資料</div>
      <header className="marketTopbar">
        <Link className="brand" href="/" aria-label="小島競標首頁">
          <span className="brandMark" aria-hidden="true"><svg viewBox="0 0 32 32"><path d="M7 20c0-6 4-11 9-11 4 0 7 3 7 7 0 5-4 9-9 9-4 0-7-2-7-5Zm15-6c2-3 5-4 7-2 2 2 1 5-2 6m-12-5h.1m6 1h.1" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg></span>
          <span>小島<span className="brandLight">競標</span></span>
        </Link>
        <nav className="marketNav" aria-label="平台導覽">
          <Link href="/auctions">真實競標</Link>
          <a className="marketNavActive" href="#auctions">展示市集</a>
          <button type="button" onClick={() => setNotice("展示模式：個人競標紀錄將於後續階段開放。")}>我的競標</button>
          <Link href="/marketplace/new">刊登商品</Link>
        </nav>
        <div className="marketUser"><span className="marketAvatar">豪</span><span><strong>展示會員</strong><small>Demo account</small></span><button type="button" aria-label="登出展示帳號" onClick={() => window.location.assign("/")}>登出</button></div>
      </header>

      <section className="marketWelcome">
        <div><p className="sectionEyebrow">THE ISLAND MARKETPLACE</p><h1>今天，想遇見誰？</h1><p>慢慢挑選，找到那個讓你心動的小生命。</p></div>
        <div className="welcomeNote"><span aria-hidden="true">✳</span><div><strong>友善競標，從了解開始</strong><small>請確認物種、來源與照護需求，再參與競標。</small></div></div>
      </section>

      <section className="marketStats" aria-label="市集摘要">
        <article><span className="statIcon statIconGreen">◉</span><div><strong>24</strong><small>進行中的競標</small></div><span className="statTrend">+3 今日新增</span></article>
        <article><span className="statIcon statIconPeach">✳</span><div><strong>08</strong><small>即將開始</small></div><span className="statTrend">最近 24 小時</span></article>
        <article><span className="statIcon statIconLilac">⌁</span><div><strong>{favorites.length.toString().padStart(2, "0")}</strong><small>我的收藏</small></div><span className="statTrend">展示資料</span></article>
      </section>

      <section className="auctionSection" id="auctions">
        <div className="auctionHeading"><div><p className="sectionEyebrow">FIND YOUR LITTLE ISLAND</p><h2>正在島上的競標</h2><p>每一個生命，都值得被好好認識。</p></div><span className="auctionCount">{filteredAuctions.length} 件展示商品</span></div>
        <div className="marketControls">
          <label className="marketSearch"><span aria-hidden="true">⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜尋物種、商品或賣家" aria-label="搜尋商品" /><kbd>⌘ K</kbd></label>
          <div className="statusTabs" aria-label="競標狀態篩選">
            {(["全部", "進行中", "即將開始"] as const).map((item) => <button key={item} type="button" className={status === item ? "statusTab statusTabActive" : "statusTab"} onClick={() => setStatus(item)}>{item}</button>)}
          </div>
        </div>
        <div className="marketBody">
          <aside className="categoryPanel"><p>探索分類</p>{categories.map((item, index) => <button key={item} type="button" className={category === item ? "categoryItem categoryItemActive" : "categoryItem"} onClick={() => setCategory(item)}><span className="categoryGlyph">{["✳", "⌁", "◉", "＋"][index]}</span>{item}<span className="categoryArrow">›</span></button>)}<div className="careCard"><span>♡</span><strong>以照護為優先</strong><p>參與競標前，請確認自己已準備好提供合適的環境與照顧。</p></div></aside>
          <div className="auctionGrid">
            {filteredAuctions.map((auction) => <article className="auctionCard" key={auction.id}>
              <div className={`auctionVisual ${auction.theme}`}><span className="auctionBadge">{auction.badge}</span><button className={favorites.includes(auction.id) ? "favoriteButton favoriteActive" : "favoriteButton"} type="button" onClick={() => toggleFavorite(auction.id)} aria-label={favorites.includes(auction.id) ? "取消收藏" : "加入收藏"}>{favorites.includes(auction.id) ? "♥" : "♡"}</button><span className="animalEmoji" aria-hidden="true">{auction.icon}</span><span className="visualCaption">ISLAND FINDS <i>✳</i></span></div>
              <div className="auctionInfo"><span className="auctionSpecies">{auction.species}</span><h3>{auction.title}</h3><div className="sellerLine"><span className="sellerAvatar">{auction.seller.slice(0, 1)}</span><span>{auction.seller}</span><span className="sellerRating">★ 4.9</span></div><div className="auctionDivider" /><div className="auctionNumbers"><div><small>目前出價</small><strong>NT$ {money.format(auction.currentPrice)}</strong><span>{auction.bids} 次出價</span></div><div className="timeLeft"><small>{auction.status === "進行中" ? "距離結標" : "開始時間"}</small><strong>{auction.remaining}</strong></div></div><Link className="bidButton" href="/auctions">前往真實競標<span aria-hidden="true">↗</span></Link></div>
            </article>)}
            {filteredAuctions.length === 0 && <div className="emptyAuctions"><span>⌕</span><strong>沒有找到符合條件的商品</strong><p>試試其他關鍵字或分類。</p></div>}
          </div>
        </div>
      </section>
      {notice && <div className="demoToast" role="status"><span>✳</span>{notice}<button type="button" onClick={() => setNotice("")} aria-label="關閉訊息">×</button></div>}
      <footer className="marketFooter"><span>小島競標・展示模式</span><span>為喜歡相遇，為生命負責。</span><Link href="/">返回首頁 ↑</Link></footer>
    </main>
  );
}
