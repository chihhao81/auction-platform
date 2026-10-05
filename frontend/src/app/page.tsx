import Link from "next/link";

function GeckoIllustration() {
  return (
    <svg className="gecko" viewBox="0 0 440 330" role="img" aria-label="守宮線條插畫">
      <defs>
        <linearGradient id="body" x1="0" x2="1" y1="0" y2="1">
          <stop offset="0" stopColor="#bfd69a" />
          <stop offset="1" stopColor="#799f73" />
        </linearGradient>
      </defs>
      <path d="M109 230c-39-16-54-52-34-79 15-20 43-22 63-7-7 19 2 38 20 46 16 7 34 1 43-13 12-19 4-45-15-53-14-6-30-3-40 8-19-12-27-34-18-56 9-21 33-32 58-27 36 7 64 38 67 75 3 42-22 73-61 81-29 6-48 20-57 43-5 12-12 23-24 33-14 11-31 13-45 5-11-7-15-19-10-30 7-15 31-22 53-26Z" fill="url(#body)" />
      <path d="M237 84c11-21 34-31 55-22 12 5 18 16 14 27-4 12-19 20-34 17-14-2-25-11-35-22Z" fill="#9fbd83" />
      <ellipse cx="280" cy="75" rx="8" ry="12" fill="#243a30" />
      <circle cx="282" cy="71" r="2.4" fill="#f8f8f3" />
      <path d="M269 92c9 6 18 6 27 0" fill="none" stroke="#50765a" strokeWidth="3" strokeLinecap="round" />
      <path d="M128 137c-24-22-53-20-65-1-9 14-2 30 13 34 11 3 23-2 29-12m46 48c-24-6-44 7-44 25 0 14 13 22 26 17 10-4 16-14 16-25m49-52c5-25 24-39 41-31 13 6 16 21 8 31-6 8-18 11-28 7" fill="none" stroke="#5f875f" strokeWidth="7" strokeLinecap="round" />
      <g fill="#eef2d8"><circle cx="127" cy="91" r="3"/><circle cx="151" cy="76" r="2.5"/><circle cx="172" cy="102" r="3"/><circle cx="112" cy="177" r="2.5"/><circle cx="184" cy="148" r="2"/><circle cx="208" cy="117" r="2.5"/><circle cx="156" cy="221" r="3"/><circle cx="201" cy="190" r="2.5"/></g>
    </svg>
  );
}

export default function Home() {
  return (
    <main className="siteShell">
      <header className="topbar">
        <a className="brand" href="#top" aria-label="小島競標首頁">
          <span className="brandMark" aria-hidden="true"><svg viewBox="0 0 32 32"><path d="M7 20c0-6 4-11 9-11 4 0 7 3 7 7 0 5-4 9-9 9-4 0-7-2-7-5Zm15-6c2-3 5-4 7-2 2 2 1 5-2 6m-12-5h.1m6 1h.1" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"/></svg></span>
          <span>小島<span className="brandLight">競標</span></span>
        </a>
        <nav className="navLinks" aria-label="主要導覽">
          <a className="navActive" href="#top">首頁</a>
          <a href="#browse">競標商品</a>
          <a href="#guide">競標說明</a>
        </nav>
          <Link className="headerLogin" href="/marketplace">進入平台 <span aria-hidden="true">↗</span></Link>
      </header>

      <section className="hero" id="top">
        <div className="heroCopy">
          <div className="eyebrow"><span className="eyebrowDot" />為每一份喜愛，找到好主人</div>
          <h1>慢慢挑選，<br /><span>安心出價。</span></h1>
          <p className="heroLead">一個為特殊寵物與收藏愛好者打造的競標空間。清楚的規則、舒服的節奏，讓每場相遇都值得期待。</p>
          <div className="sessionPanel" id="login">
            <Link className="cta" href="/marketplace">使用展示帳號登入 <span aria-hidden="true">↗</span></Link>
            <p className="sessionIntro">展示模式會直接進入平台主畫面，不會呼叫 LINE 或後端登入。</p>
          </div>
          <div className="heroNotes"><span><i aria-hidden="true">✓</i> 出價規則清楚</span><span><i aria-hidden="true">✓</i> 交易對話留在平台</span></div>
        </div>
        <div className="heroArtwork" aria-hidden="true">
          <div className="sunDisc" />
          <div className="leaf leafOne" /><div className="leaf leafTwo" />
          <div className="artCaption"><span className="captionKicker">A LITTLE ISLAND</span><span>給生命多一點<br />被好好對待的機會。</span></div>
          <GeckoIllustration />
          <div className="floatTag"><span className="tagIcon">✳</span><span><b>溫柔競標</b><small>從清楚開始</small></span></div>
        </div>
      </section>

      <section className="browseSection" id="browse">
        <div className="sectionHeading"><div><p className="sectionEyebrow">THE MARKETPLACE</p><h2>先來逛逛小島</h2></div><span className="phaseBadge"><i /> 展示模式</span></div>
        <div className="browseCard">
          <div className="browseIcon" aria-hidden="true"><svg viewBox="0 0 48 48"><path d="M10 31c-2-8 3-17 12-19 8-2 15 3 16 11 2 8-4 15-12 16-7 1-14-2-16-8Zm6-13c3-3 7-4 10-2m8 2h.1m-9 13c3 1 6 0 8-2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></svg></div>
          <div className="browseCopy"><h3>市集主畫面已開放預覽</h3><p>點擊上方按鈕即可直接進入展示平台，瀏覽模擬商品、搜尋及切換分類。LINE 登入和真實競標資料會在前後端部署後接上。</p></div>
          <Link className="textLink" href="/marketplace">進入市集 <span aria-hidden="true">→</span></Link>
        </div>
      </section>

      <section className="guideSection" id="guide">
        <div className="guideIntro"><p className="sectionEyebrow">A SIMPLE, THOUGHTFUL WAY</p><h2>每一場競標，<br />從信任開始。</h2><p>小島希望讓喜歡活體與特殊寵物的人，能在清楚的規則裡交流與交易。</p></div>
        <div className="guideSteps">
          <article className="guideStep"><span className="stepNo">01</span><div><h3>先認識彼此</h3><p>目前使用展示帳號直接進入；部署完成後再啟用 LINE 登入。</p></div><span className="stepGlyph" aria-hidden="true">◌</span></article>
          <article className="guideStep"><span className="stepNo">02</span><div><h3>看清楚每個細節</h3><p>商品資訊、出價範圍與競標時間都會清楚呈現。</p></div><span className="stepGlyph" aria-hidden="true">⌕</span></article>
          <article className="guideStep"><span className="stepNo">03</span><div><h3>讓交易有跡可循</h3><p>得標後透過平台訊息聯絡，逐步完成交易。</p></div><span className="stepGlyph" aria-hidden="true">↗</span></article>
        </div>
      </section>

      <section className="noteSection">
        <span className="noteMark" aria-hidden="true">✳</span><div><p className="sectionEyebrow">A NOTE ON LIVING CREATURES</p><h2>生命不是普通的商品。</h2><p>刊登與交易活體商品時，請先確認物種、來源、照護及運送方式符合相關法令與動物福利要求。平台規則不取代法定責任。</p></div>
      </section>

      <footer className="footer"><a className="brand footerBrand" href="#top"><span className="brandMark" aria-hidden="true">島</span><span>小島<span className="brandLight">競標</span></span></a><span>為喜歡相遇，為生命負責。</span><a href="#login">回到登入 <span aria-hidden="true">↑</span></a></footer>
    </main>
  );
}
