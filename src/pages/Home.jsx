import React, { useState, useEffect, useCallback, useRef, memo } from 'react';
import { useAuction } from '../context/AuctionContext';
import { useAuth } from '../context/AuthContext';
import './Home.css';

const Home = () => {
    const { auctions, placeBid, fetchAuctions, fetchAuctionBidsDetail, loading: auctionLoading } = useAuction();
    const { user } = useAuth();
    const [cooldown, setCooldown] = useState({});
    const [historyModal, setHistoryModal] = useState({ show: false, bids: [], auctionName: '', loading: false });
    const [descModal, setDescModal] = useState({ show: false, content: '', title: '' });
    const [bidModal, setBidModal] = useState({ show: false, auctionId: null, auctionName: '', minBid: 0 });
    const [verifyModal, setVerifyModal] = useState({ show: false });
    const [imgModal, setImgModal] = useState({ show: false, src: '' });

    const handleShowHistory = useCallback(async (auctionId, auctionName) => {
        setHistoryModal(prev => ({ ...prev, show: true, auctionName, bids: [], loading: true }));
        try {
            const fullBids = await fetchAuctionBidsDetail(auctionId);
            setHistoryModal(prev => ({ ...prev, bids: fullBids, loading: false }));
        } catch (err) {
            console.error('Failed to fetch history:', err);
            setHistoryModal(prev => ({ ...prev, show: false, loading: false }));
            alert('無法取得完整紀錄，請稍後再試');
        }
    }, [fetchAuctionBidsDetail]);

    const maskEmail = (email) => {
        if (!email) return '系統用戶';
        // If it's a UUID (guest/placeholder), mask it or show generic name
        if (email.length > 30) return '用戶 ' + email.slice(0, 5) + '...';
        const parts = email.split('@');
        if (parts.length < 2) return email;
        if (parts[0].length <= 5) return '*****@' + parts[1];
        return parts[0].slice(0, -5) + '*****@' + parts[1];
    };

    const handleBid = useCallback((auctionId, minIncrement, currentBids) => {
        if (!user?.is_verified) {
            setVerifyModal({ show: true });
            return;
        }

        if (cooldown[auctionId]) {
            alert('深呼吸一下');
            return;
        }

        const auction = auctions.find(a => a.id === auctionId);
        const currentPrice = currentBids.length > 0 ? currentBids[0].amount : auction.startPrice;
        const minBid = currentPrice + minIncrement;

        setBidModal({
            show: true,
            auctionId,
            auctionName: auction.name,
            minBid
        });
    }, [user?.is_verified, cooldown, auctions]);

    const submitBid = async (amount) => {
        const { auctionId } = bidModal;
        try {
            await placeBid(auctionId, Number(amount));
            await fetchAuctions();

            setCooldown(prev => ({ ...prev, [auctionId]: true }));
            setTimeout(() => {
                setCooldown(prev => ({ ...prev, [auctionId]: false }));
            }, 5000);
            setBidModal({ ...bidModal, show: false });
        } catch (err) {
            alert(err.message || '出價失敗，請稍後再試');
        }
    };

    if (auctionLoading) return <div className="loading-container">讀取中...</div>;

    return (
        <div className="home-container">
            {auctions.length === 0 ? (
                <div className="no-products-container glass-card">
                    <p>目前沒有競標中的商品</p>
                </div>
            ) : (
                <div className="product-grid">
                    {auctions.map(auction => {
                        const currentPrice = auction.bids.length > 0 ? auction.bids[0].amount : auction.startPrice;
                        const parseUTC = (dateStr) => {
                            if (!dateStr) return new Date();
                            if (dateStr.includes('Z')) return new Date(dateStr);
                            return new Date(dateStr.replace(' ', 'T') + 'Z');
                        };
                        const isEnded = auction.status === 'ended' || parseUTC(auction.endTime) < new Date();
                        const isUpcoming = !isEnded && (auction.status === 'upcoming' || parseUTC(auction.startTime) > new Date());

                        return (
                            <AuctionCard
                                key={auction.id}
                                auction={auction}
                                user={user}
                                isUpcoming={isUpcoming}
                                isEnded={isEnded}
                                handleBid={handleBid}
                                cooldown={cooldown}
                                onShowHistory={handleShowHistory}
                                setDescModal={setDescModal}
                                onImageClick={(src) => setImgModal({ show: true, src })}
                            />
                        );
                    })}
                </div>
            )}

            {historyModal.show && (
                <HistoryModal
                    auctionName={historyModal.auctionName}
                    bids={historyModal.bids}
                    loading={historyModal.loading}
                    onClose={() => setHistoryModal({ ...historyModal, show: false })}
                />
            )}

            {descModal.show && (
                <div className="modal-overlay" onClick={() => setDescModal({ ...descModal, show: false })}>
                    <div className="modal-content glass-card desc-modal" onClick={e => e.stopPropagation()}>
                        <div className="modal-header">
                            <h3>{descModal.title} - 商品詳情</h3>
                            <button className="close-btn" onClick={() => setDescModal({ ...descModal, show: false })}>&times;</button>
                        </div>
                        <div className="modal-body">
                            <div className="full-description">
                                {descModal.content}
                            </div>
                        </div>
                        <div className="modal-footer">
                            <button className="btn-primary" onClick={() => setDescModal({ ...descModal, show: false })}>關閉</button>
                        </div>
                    </div>
                </div>
            )}

            {bidModal.show && (
                <BidModal
                    auctionName={bidModal.auctionName}
                    minBid={bidModal.minBid}
                    onClose={() => setBidModal({ ...bidModal, show: false })}
                    onSubmit={submitBid}
                />
            )}

            {verifyModal.show && (
                <div className="modal-overlay" onClick={() => setVerifyModal({ show: false })}>
                    <div className="modal-content glass-card desc-modal" onClick={e => e.stopPropagation()} style={{ maxWidth: '450px', textAlign: 'center' }}>
                        <div className="modal-header">
                            <h3 style={{ color: '#ff4d4f' }}>驗證提示</h3>
                            <button className="close-btn" onClick={() => setVerifyModal({ show: false })}>&times;</button>
                        </div>
                        <div className="modal-body">
                            <h4 style={{ marginBottom: '1rem', fontSize: '1.2rem', color: '#fff' }}>您尚未通過人工驗證，請聯繫Line官方帳號。</h4>
                            <div style={{ background: 'rgba(255,255,255,0.05)', padding: '1.5rem', borderRadius: '12px', margin: '1rem 0' }}>
                                <p style={{ marginBottom: '1rem', fontSize: '1.1rem' }}>
                                    <a href="https://lin.ee/29HRA8aF" target="_blank" rel="noreferrer" style={{ color: '#00B900', textDecoration: 'none', fontWeight: 'bold' }}>
                                        👉 加入連結 | https://lin.ee/29HRA8aF
                                    </a>
                                </p>
                                <p style={{ marginBottom: '0.5rem', color: '#ccc' }}>或掃QR code</p>
                                <img src="https://qr-official.line.me/gs/M_056qctjm_GW.png" alt="Line QR Code" style={{ width: '150px', height: '150px', borderRadius: '8px', marginBottom: '1rem', background: '#fff', padding: '5px' }} />
                            </div>
                            <p style={{ color: '#888', fontSize: '0.9rem' }}>如果連接失效，請直接搜尋Line ID <strong style={{ color: '#fff' }}>@056qctjm</strong></p>
                        </div>
                        <div className="modal-footer" style={{ justifyContent: 'center' }}>
                            <button className="btn-primary" onClick={() => setVerifyModal({ show: false })}>我知道了</button>
                        </div>
                    </div>
                </div>
            )}

            {imgModal.show && (
                <div className="modal-overlay image-modal-overlay" onClick={() => setImgModal({ show: false, src: '' })}>
                    <div className="image-modal-content" onClick={e => e.stopPropagation()}>
                        <img src={imgModal.src} alt="Full size" className="full-image-preview" />
                        <button className="close-btn circle" onClick={() => setImgModal({ show: false, src: '' })}>&times;</button>
                    </div>
                </div>
            )}
        </div>
            );
        };
        
        const AuctionCard = memo(({ auction, user, isUpcoming, isEnded, handleBid, cooldown, onShowHistory, setDescModal, onImageClick }) => {
    const currentPrice = auction.bids.length > 0 ? auction.bids[0].amount : auction.startPrice;
    const [isTension, setIsTension] = useState(false);
    const descRef = useRef(null);
    const [isOverflow, setIsOverflow] = useState(false);

    useEffect(() => {
        const el = descRef.current;
        if (el) {
            setIsOverflow(el.scrollHeight > el.clientHeight);
        }
    }, [auction.description]);

    return (
        <div id={`auction-${auction.id}`} className={`product-card glass-card ${isUpcoming ? 'is-upcoming' : ''} ${isEnded ? 'is-ended' : ''} ${isTension ? 'tension-pulse' : ''}`}>
            <div 
                className="product-image" 
                style={{ backgroundImage: `url(${auction.image})`, cursor: 'pointer' }}
                onClick={() => onImageClick(auction.image)}
            >
                {isUpcoming && <div className="status-overlay upcoming">即將開始</div>}
                {isEnded && <div className="status-overlay ended">已結束</div>}
            </div>
            <div className="product-info">
                <h3>{auction.name}</h3>
                {auction.description && (
                    <div
                        className={`product-description ${isOverflow ? 'clickable' : ''}`}
                        onClick={() => isOverflow && setDescModal({ show: true, content: auction.description, title: auction.name })}
                    >
                        <div className="desc-text" ref={descRef}>
                            {auction.description}
                        </div>
                        {isOverflow && <span className="desc-show-more">顯示全部</span>}
                    </div>
                )}
                <div className="price-info">
                    <span className="label">
                        {isUpcoming ? '起標價格' : (isEnded ? '得標價' : '目前最高價')}
                    </span>
                    <span className="amount">${currentPrice}</span>
                </div>
                <div className="meta-info">
                    {isUpcoming && <span>開始時間: {new Date(auction.startTime).toLocaleString()}</span>}
                    {!isEnded && !isUpcoming && (
                        <div className="countdown-wrapper">
                            <span className="label">剩餘時間: </span>
                            <CountdownTimer
                                endTime={auction.endTime}
                                onTensionChange={setIsTension}
                            />
                        </div>
                    )}
                    <span>結束時間: {new Date(auction.endTime).toLocaleString()}</span>
                    <span>最低加價: ${auction.minIncrement}</span>
                </div>

                {isEnded && auction.bids.length > 0 && (
                    <div className="winner-announcement">
                        <p>🏆 恭喜：{auction.bids[0].line_group_name}</p>
                    </div>
                )}

                {user && !user.isAdmin && !isEnded && !isUpcoming && (
                    <button
                        className="btn-primary bid-btn"
                        onClick={() => handleBid(auction.id, auction.minIncrement, auction.bids)}
                        disabled={cooldown[auction.id]}
                    >
                        {cooldown[auction.id] ? '深呼吸一下' : '我要出價'}
                    </button>
                )}

                <div className="bid-history">
                    <h4>出價紀錄</h4>
                    <BidHistory
                        bids={auction.bids}
                        totalCount={auction.bid_count}
                        onShowAll={() => onShowHistory(auction.id, auction.name)}
                    />
                </div>
            </div>
        </div>
    );
}, (prevProps, nextProps) => {
    return (
        prevProps.auction.id === nextProps.auction.id &&
        prevProps.auction.bids.length === nextProps.auction.bids.length &&
        prevProps.auction.bid_count === nextProps.auction.bid_count && // 補上 bid_count 比較
        prevProps.auction.status === nextProps.auction.status &&
        prevProps.isUpcoming === nextProps.isUpcoming &&
        prevProps.isEnded === nextProps.isEnded &&
        prevProps.cooldown[prevProps.auction.id] === nextProps.cooldown[nextProps.auction.id] &&
        prevProps.onShowHistory === nextProps.onShowHistory
    );
});

const CountdownTimer = ({ endTime, onTensionChange }) => {
    const [timeLeft, setTimeLeft] = useState('');

    useEffect(() => {
        const updateTimer = () => {
            const now = new Date();
            const end = new Date(endTime.replace(' ', 'T') + (endTime.includes('Z') ? '' : 'Z'));
            const diff = end - now;

            if (diff <= 0) {
                setTimeLeft('已結束');
                onTensionChange(false);
                return;
            }

            // Check for tension (less than 1 minute)
            if (diff < 60000) {
                onTensionChange(true);
            } else {
                onTensionChange(false);
            }

            const days = Math.floor(diff / (1000 * 60 * 60 * 24));
            const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
            const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
            const seconds = Math.floor((diff % (1000 * 60)) / 1000);

            let timeStr = '';
            if (days > 0) timeStr += `${days}天 `;
            timeStr += `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
            setTimeLeft(timeStr);
        };

        updateTimer();
        const interval = setInterval(updateTimer, 1000);
        return () => clearInterval(interval);
    }, [endTime, onTensionChange]);

    return <span className={`timer-text ${timeLeft === '已結束' ? 'ended' : ''}`}>{timeLeft}</span>;
};

const BidModal = ({ auctionName, minBid, onClose, onSubmit }) => {
    const [amount, setAmount] = useState(minBid);

    const handleSubmit = (e) => {
        e.preventDefault();
        if (Number(amount) < minBid) {
            alert('金額不足！');
            return;
        }
        if (Number(amount) > 99999999) {
            alert('你出這價我人都可以考慮賣你了');
            return;
        }
        onSubmit(amount);
    };

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content glass-card" onClick={e => e.stopPropagation()} style={{ maxWidth: '400px' }}>
                <div className="modal-header">
                    <h3>我要出價</h3>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>
                <form onSubmit={handleSubmit}>
                    <div className="modal-body">
                        <p style={{ marginBottom: '1rem' }}>商品：<strong>{auctionName}</strong></p>
                        <div className="input-group">
                            <label>請輸入出價金額 (最低 ${minBid})</label>
                            <input
                                type="number"
                                inputMode="decimal"
                                pattern="[0-9]*"
                                value={amount}
                                onChange={(e) => setAmount(e.target.value)}
                                onWheel={(e) => e.target.blur()} // 禁用滾輪調節
                                min={minBid}
                                max={99999999}
                                required
                                autoFocus
                                style={{ fontSize: '1.2rem', padding: '0.8rem' }}
                            />
                            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.8rem' }}>
                                {[50, 100, 500, 1000].map(val => (
                                    <button 
                                        key={val} 
                                        type="button" 
                                        onClick={() => setAmount(prev => Number(prev) + val)} 
                                        style={{ 
                                            flex: 1, 
                                            padding: '0.5rem 0', 
                                            fontSize: '1rem', 
                                            fontWeight: 'bold',
                                            border: '1px solid rgba(255, 255, 255, 0.3)',
                                            borderRadius: '8px',
                                            backgroundColor: 'rgba(255, 255, 255, 0.08)',
                                            color: '#fff',
                                            cursor: 'pointer',
                                            transition: 'all 0.2s'
                                        }}
                                        onMouseOver={(e) => { e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.15)'; e.currentTarget.style.borderColor = 'rgba(255, 255, 255, 0.5)'; }}
                                        onMouseOut={(e) => { e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.08)'; e.currentTarget.style.borderColor = 'rgba(255, 255, 255, 0.3)'; }}
                                    >
                                        +{val}
                                    </button>
                                ))}
                            </div>
                        </div>
                    </div>
                    <div className="modal-footer" style={{ gap: '1rem', display: 'flex' }}>
                        <button type="button" className="btn-secondary" onClick={onClose} style={{ flex: 1 }}>取消</button>
                        <button type="submit" className="btn-primary" style={{ flex: 1 }}>確認出價</button>
                    </div>
                </form>
            </div>
        </div>
    );
};

const BidHistory = memo(({ bids, totalCount, onShowAll }) => {
    const displayBids = bids.slice(0, 3);

    if (bids.length === 0) return <p className="no-bids">暫無出價</p>;

    return (
        <div className="history-list-wrapper">
            <div className="history-list">
                {displayBids.map((bid, i) => (
                    <div key={bid.id || i} className="history-item">
                        <span className="bidder">
                            <span className="group-name-only">{bid.line_group_name}</span>
                        </span>
                        <span className="time">{new Date(bid.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                        <span className="price">${bid.amount}</span>
                    </div>
                ))}
            </div>
            {(totalCount > 0 || bids.length > 0) && (
                <button
                    className="toggle-history-btn"
                    onClick={onShowAll}
                >
                    顯示完整紀錄 ({totalCount || bids.length})
                </button>
            )}
        </div>
    );
}, (prevProps, nextProps) => {
    return prevProps.bids.length === nextProps.bids.length && prevProps.totalCount === nextProps.totalCount;
});

const HistoryModal = ({ auctionName, bids, loading, onClose }) => {
    const [visibleCount, setVisibleCount] = useState(20);
    const scrollTimeoutRef = React.useRef(null);

    const handleScroll = useCallback((e) => {
        if (scrollTimeoutRef.current) return; // Throttle scrolling

        const { scrollTop, scrollHeight, clientHeight } = e.target;

        scrollTimeoutRef.current = setTimeout(() => {
            if (scrollHeight - scrollTop <= clientHeight + 150) {
                if (visibleCount < bids.length) {
                    setVisibleCount(prev => Math.min(prev + 20, bids.length));
                }
            }
            scrollTimeoutRef.current = null;
        }, 100); // Check every 100ms max
    }, [visibleCount, bids.length]);

    // Use memo to prevent re-rendering identical rows
    const renderedBids = React.useMemo(() => {
        return bids.slice(0, visibleCount).map((bid, i) => (
            <HistoryRow key={bid.id || i} bid={bid} />
        ));
    }, [bids, visibleCount]);

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content glass-card history-modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h3>{auctionName} - 完整出價紀錄</h3>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>
                <div className="modal-body" onScroll={handleScroll} style={{ overflowY: 'auto', maxHeight: '60vh', position: 'relative' }}>
                    {loading ? (
                        <div style={{ padding: '2rem', textAlign: 'center' }}>載入中...</div>
                    ) : (
                        <table className="history-table">
                            <thead>
                                <tr>
                                    <th>出價者</th>
                                    <th>時間</th>
                                    <th>金額</th>
                                </tr>
                            </thead>
                            <tbody>
                                {renderedBids}
                            </tbody>
                        </table>
                    )}
                    {visibleCount < bids.length && (
                        <div style={{ textAlign: 'center', padding: '10px 0', color: '#888' }}>向下滾動加載更多...</div>
                    )}
                </div>
                <div className="modal-footer">
                    <button className="btn-primary" onClick={onClose}>關閉</button>
                </div>
            </div>
        </div>
    );
};

const HistoryRow = memo(({ bid }) => {
    return (
        <tr>
            <td>
                <div className="bidder-info">
                    <span className="group-name-detail">{bid.line_group_name}</span>
                </div>
            </td>
            <td>{new Date(bid.time).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</td>
            <td className="price-detail">${bid.amount}</td>
        </tr>
    );
});

export default Home;
