import React, { createContext, useContext, useState, useEffect } from 'react';
import { supabase } from '../utils/supabaseClient';
import { useAuth } from './AuthContext';
import { SUPABASE_URL } from '../utils/envConfig';
import { ensureUTC, sortAuctions } from '../utils/auctionUtils';

const AuctionContext = createContext();

export const AuctionProvider = ({ children }) => {
    const [auctions, setAuctions] = useState([]);
    const [loading, setLoading] = useState(true);
    const [notifications, setNotifications] = useState([]);
    const { user } = useAuth();
    const notifiedWonAuctions = React.useRef(new Set());
    const notifiedOutbids = React.useRef(new Set());
    const profilesCache = React.useRef(new Map());

    const playNotificationSound = () => {
        const audio = new Audio('https://assets.mixkit.co/active_storage/sfx/2869/2869-preview.mp3');
        audio.volume = 0.5;
        audio.play().catch(e => console.log('Sound play error:', e));
    };

    const addNotification = (message, type = 'info', auctionId = null) => {
        const id = Date.now().toString() + Math.random().toString(36).substring(2, 9);
        setNotifications(prev => {
            const updated = [...prev, { id, message, type, auctionId }];
            return updated.slice(-3); // 只保留最後 3 個通知
        });
        playNotificationSound();
    };

    const removeNotification = (id) => {
        setNotifications(prev => prev.filter(n => n.id !== id));
    };


    const formatAuctionsData = (auctionsData) => {
        const STORAGE_URL = `${SUPABASE_URL}/storage/v1/object/public/product_images`;

        return auctionsData.map(a => ({
            id: a.id,
            name: a.title,
            description: a.description,
            image: `${STORAGE_URL}/${a.product_id}.jpg?t=${new Date(a.created_at).getTime()}`,
            startPrice: a.start_price,
            minIncrement: a.min_increment,
            startTime: ensureUTC(a.start_time),
            endTime: ensureUTC(a.end_time),
            createdAt: ensureUTC(a.created_at),
            status: a.status,
            product_id: a.product_id,
            highest_bid: a.highest_bid,
            bid_count: a.bid_count,
            top_bidder_id: a.top_bidder_id,
            bids: (a.top_bids || []).map(b => ({
                id: Math.random().toString(36).substring(7),
                email: b.user_id || 'guest',
                line_group_name: b.bidder_name || 'unknown',
                amount: b.bid_amount,
                time: ensureUTC(b.created_at)
            }))
        }));
    };

    const fetchRawData = async () => {
        const { data: auctionsData, error: auctionError } = await supabase
            .from('auctions_with_top_bids')
            .select('id, title, description, product_id, start_price, min_increment, start_time, end_time, created_at, status, highest_bid, bid_count, top_bids, top_bidder_id');

        if (auctionError) throw auctionError;

        return formatAuctionsData(auctionsData);
    };

    const fetchAuctionBidsDetail = async (auctionId) => {
        const { data, error } = await supabase
            .from('auction_bids_detail')
            .select('*')
            .eq('auction_id', auctionId)
            .order('bid_amount', { ascending: false });

        if (error) throw error;

        return data.map(b => ({
            id: b.bid_id, // 這裡是 bid_id
            email: b.user_id,
            line_group_name: b.bidder_name || 'unknown',
            amount: b.bid_amount,
            time: ensureUTC(b.created_at)
        }));
    };

    const fetchAuctions = async () => {
        setLoading(true);
        try {
            const formatted = await fetchRawData();
            setAuctions(sortAuctions(formatted));
        } catch (error) {
            console.error('Error fetching auctions:', error);
        } finally {
            setLoading(false);
        }
    };

    const auctionsRef = React.useRef(auctions);
    useEffect(() => {
        auctionsRef.current = auctions;
    }, [auctions]);

    const lastFetchedAuctionsUserRef = React.useRef(null);
    useEffect(() => {
        if (!user) {
            setAuctions([]);
            setLoading(false);
            lastFetchedAuctionsUserRef.current = null;
            return;
        }

        // 只有當 User ID 改變時，才進行初始的全量抓取
        if (lastFetchedAuctionsUserRef.current !== user.id) {
            lastFetchedAuctionsUserRef.current = user.id;
            fetchAuctions();
        }

        // 無論如何（包括 StrictMode 的 Remount）都要啟動輪詢，因為舊的已被 Cleanup 清除
        const intervalId = setInterval(async () => {
            try {
                const freshAuctions = await fetchRawData();
                const oldAuctions = auctionsRef.current;

                if (user) {
                    for (const freshA of freshAuctions) {
                        const oldA = oldAuctions.find(a => a.id === freshA.id);
                        if (!oldA) continue;

                        const bidCountChanged = freshA.bid_count !== oldA.bid_count;
                        const wasTopBidder = oldA.top_bidder_id === user.id;
                        const isStillTopBidder = freshA.top_bidder_id === user.id;

                        if (bidCountChanged && wasTopBidder && !isStillTopBidder) {
                            addNotification(`通知：拍賣「${freshA.name}」有了新的出價 $${freshA.highest_bid}，您已被超標！`, 'warning', freshA.id);
                        }

                        if (freshA.status === 'ended' && oldA.status !== 'ended') {
                            if (freshA.top_bidder_id === user.id) {
                                if (!notifiedWonAuctions.current.has(freshA.id)) {
                                    notifiedWonAuctions.current.add(freshA.id);
                                    addNotification(`恭喜！您已成功得標拍賣「${freshA.name}」！請盡速私訊官方帳號進行後續處理。`, 'success');
                                }
                            }
                        }
                    }
                }

                setAuctions(sortAuctions(freshAuctions));
            } catch (error) {
                console.error('Polling error:', error);
            }
        }, 5000);

        return () => clearInterval(intervalId);
    }, [user?.id]);

    const addProduct = async (product) => {
        // Implementation for admin
    };

    const placeBid = async (auctionId, amount) => {
        const { error } = await supabase.rpc('place_bid', {
            p_auction_id: auctionId,
            p_amount: amount
        });

        if (error) {
            console.error('Error in place_bid RPC:', error);
            // 發生出價錯誤（如：價格已被更新），立刻重新拉取最新資料
            await fetchAuctions();
            // Throw the actual message from Postgres (e.g. "You are already the highest bidder")
            throw new Error(error.message || '出價失敗');
        }

        // 出價成功也主動更新一下，讓 UI 立即反應
        await fetchAuctions();
    };

    return (
        <AuctionContext.Provider value={{
            auctions,
            loading,
            fetchAuctions,
            fetchAuctionBidsDetail,
            addProduct,
            placeBid,
            notifications,
            removeNotification
        }}>
            {children}
            <div className="toast-container">
                {notifications.map(n => (
                    <div
                        key={n.id}
                        className={`toast-item ${n.type}`}
                        onClick={() => {
                            if (n.auctionId) {
                                const el = document.getElementById(`auction-${n.auctionId}`);
                                if (el) {
                                    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                                    // 增加一個短暫的高亮效果或移除通知
                                    removeNotification(n.id);
                                }
                            }
                        }}
                    >
                        <div className="toast-content">{n.message}</div>
                        <button
                            className="toast-close"
                            onClick={(e) => {
                                e.stopPropagation();
                                removeNotification(n.id);
                            }}
                        >
                            &times;
                        </button>
                    </div>
                ))}
            </div>
        </AuctionContext.Provider>
    );
};

export const useAuction = () => useContext(AuctionContext);
