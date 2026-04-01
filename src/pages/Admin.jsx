import React, { useState, useEffect } from 'react';
import { useAuction } from '../context/AuctionContext';
import { supabase } from '../utils/supabaseClient';
import './Admin.css';

export const PRODUCT_SIZES = [
    { id: '0', value: '0.3cm以上' },
    { id: '1', value: '0.5cm以上' },
    { id: '2', value: '亞成成體' },
    { id: '3', value: '幼體' },
    { id: '4', value: '小亞成' },
    { id: '5', value: '0.5-1公分' },
    { id: '6', value: '1-1.5公分' },
    { id: '7', value: '1.5-2公分' },
    { id: '8', value: '2-2.5公分' },
    { id: '9', value: '3公分以上' },
];

const Admin = () => {
    const { addProduct, fetchAuctions } = useAuction();
    const [productsList, setProductsList] = useState([]);
    const [loadingProducts, setLoadingProducts] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [showDropdown, setShowDropdown] = useState(false);

    const getCurrentTime = () => {
        const now = new Date();
        const nextStart = new Date(now);
        nextStart.setMilliseconds(0);
        nextStart.setSeconds(0);
        nextStart.setMinutes(0);

        if (now.getMinutes() <= 30) {
            nextStart.setHours(now.getHours() + 1);
        } else {
            nextStart.setHours(now.getHours() + 2);
        }

        const year = nextStart.getFullYear();
        const month = String(nextStart.getMonth() + 1).padStart(2, '0');
        const day = String(nextStart.getDate()).padStart(2, '0');
        const hours = String(nextStart.getHours()).padStart(2, '0');
        const minutes = String(nextStart.getMinutes()).padStart(2, '0');
        return `${year}-${month}-${day}T${hours}:${minutes}`;
    };

    const getDefaultEndTime = () => {
        const now = new Date();
        const tomorrow = new Date(now);
        tomorrow.setDate(tomorrow.getDate() + 1);
        tomorrow.setHours(22, 30, 0, 0);

        const year = tomorrow.getFullYear();
        const month = String(tomorrow.getMonth() + 1).padStart(2, '0');
        const day = String(tomorrow.getDate()).padStart(2, '0');
        const hours = String(tomorrow.getHours()).padStart(2, '0');
        const minutes = String(tomorrow.getMinutes()).padStart(2, '0');
        return `${year}-${month}-${day}T${hours}:${minutes}`;
    };

    const [formData, setFormData] = useState({
        productId: '',
        title: '',
        description: localStorage.getItem('aura_last_description') || '',
        startPrice: '0',
        minIncrement: '50',
        startTime: getCurrentTime(),
        endTime: getDefaultEndTime(),
        productSize: '0',
        quantity: 1,
        paymentAccountId: localStorage.getItem('aura_last_payment_account') || ''
    });

    const [paymentAccountsList, setPaymentAccountsList] = useState([]);
    const [loadingAccounts, setLoadingAccounts] = useState(false);

    const fetchPaymentAccounts = async (force = false) => {
        if (!force) {
            const cached = localStorage.getItem('aura_payment_accounts_cache');
            if (cached) {
                try {
                    setPaymentAccountsList(JSON.parse(cached));
                    return;
                } catch (e) {
                    console.error('解析匯款帳號快取失敗:', e);
                }
            }
        }

        setLoadingAccounts(true);
        try {
            const { data, error } = await supabase
                .from('payment_accounts')
                .select('*')
                .eq('is_active', true);

            if (error) throw error;
            setPaymentAccountsList(data || []);
            localStorage.setItem('aura_payment_accounts_cache', JSON.stringify(data || []));

            if (data && data.length > 0) {
                const currentSaved = localStorage.getItem('aura_last_payment_account');
                const exists = data.find(acc => acc.id === currentSaved);
                if (!exists) {
                    setFormData(prev => ({ ...prev, paymentAccountId: data[0].id }));
                }
            }
        } catch (err) {
            console.error('獲取匯款帳號清單失敗:', err);
            alert('匯款帳號清單獲取失敗。');
        } finally {
            setLoadingAccounts(false);
        }
    };

    const fetchProducts = async (force = false) => {
        // Try to load from cache first if not forced
        if (!force) {
            const cached = localStorage.getItem('aura_products_cache');
            if (cached) {
                try {
                    setProductsList(JSON.parse(cached));
                    return;
                } catch (e) {
                    console.error('解析快取失敗:', e);
                }
            }
        }

        setLoadingProducts(true);
        try {
            const { data, error } = await supabase.functions.invoke("get-products", {
                headers: {
                    Authorization: `Bearer ${import.meta.env.VITE_SUPABASE_ANON_KEY}`,
                },
            });
            if (error) throw error;
            setProductsList(data || []);
            localStorage.setItem('aura_products_cache', JSON.stringify(data || []));
        } catch (err) {
            console.error('獲取產品清單失敗:', err);
            alert('產品清單獲取失敗，請確認網路或後端 CORS 設定。');
        } finally {
            setLoadingProducts(false);
        }
    };

    useEffect(() => {
        fetchProducts();
        fetchPaymentAccounts();

        // Close dropdown when clicking outside
        const handleClickOutside = (e) => {
            if (!e.target.closest('.autocomplete-container')) {
                setShowDropdown(false);
            }
        };
        document.addEventListener('click', handleClickOutside);
        return () => document.removeEventListener('click', handleClickOutside);
    }, []);

    const filteredProducts = productsList.filter(p =>
        p.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
        p.name.toLowerCase().includes(searchTerm.toLowerCase())
    );

    const handleProductSelect = (product) => {
        setFormData({
            ...formData,
            productId: product.id,
            title: product.name,
        });
        setSearchTerm(`${product.id} - ${product.name}`);
        setShowDropdown(false);
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!formData.productId) {
            alert('請選擇產品');
            return;
        }
        if (!formData.paymentAccountId) {
            alert('請選擇匯款帳號');
            return;
        }

        const parsedQuantity = parseInt(formData.quantity, 10);
        if (isNaN(parsedQuantity) || parsedQuantity < 1) {
            alert('請輸入有效的數量 (至少為 1)');
            return;
        }

        setSubmitting(true);
        try {
            const finalStartTime = formData.startTime ? new Date(formData.startTime) : new Date();
            const finalEndTime = new Date(formData.endTime);

            if (formData.paymentAccountId) {
                localStorage.setItem('aura_last_payment_account', formData.paymentAccountId);
            }
            localStorage.setItem('aura_last_description', formData.description || '');

            const { error } = await supabase.rpc('create_auction', {
                p_description: formData.description || '',
                p_start_price: Number(formData.startPrice),
                p_min_increment: Number(formData.minIncrement),
                p_start_time: finalStartTime.toISOString(),
                p_end_time: finalEndTime.toISOString(),
                p_product_id: String(formData.productId),
                p_product_name: String(formData.title),
                p_product_size: String(formData.productSize),
                p_quantity: Number(formData.quantity),
                p_payment_account_id: String(formData.paymentAccountId)
            });
            if (error) throw error;

            setFormData({
                productId: '',
                title: '',
                description: localStorage.getItem('aura_last_description') || '',
                startPrice: '0',
                minIncrement: '50',
                startTime: getCurrentTime(),
                endTime: getDefaultEndTime(),
                productSize: '0',
                quantity: 1,
                paymentAccountId: localStorage.getItem('aura_last_payment_account') || ''
            });
            setSearchTerm('');
            alert('產品發佈成功！');
            fetchAuctions();
        } catch (error) {
            alert('發佈失敗: ' + error.message);
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="admin-container">
            <form className="admin-card glass-card" onSubmit={handleSubmit}>
                <h2>新增競標產品</h2>

                <div className="input-group">
                    <label>產品 ({filteredProducts.length})</label>
                    <div className="product-selection-header">
                        <div className="autocomplete-container">
                            <input
                                type="text"
                                className="search-input"
                                placeholder="請輸入產品 ID 或名稱搜尋..."
                                value={searchTerm}
                                onChange={(e) => {
                                    setSearchTerm(e.target.value);
                                    setShowDropdown(true);
                                }}
                                onFocus={() => setShowDropdown(true)}
                            />
                            {showDropdown && (
                                <div className="dropdown-list">
                                    {filteredProducts.length > 0 ? (
                                        filteredProducts.map(p => (
                                            <div
                                                key={p.id}
                                                className="dropdown-item"
                                                onClick={() => handleProductSelect(p)}
                                            >
                                                {p.id} - {p.name}
                                            </div>
                                        ))
                                    ) : (
                                        <div className="dropdown-item no-results">找無相關產品</div>
                                    )}
                                </div>
                            )}
                        </div>
                        <button
                            type="button"
                            className="refresh-btn"
                            onClick={() => fetchProducts(true)}
                            disabled={loadingProducts}
                            title="重新整理清單"
                        >
                            {loadingProducts ? '...' : '刷新'}
                        </button>
                    </div>
                </div>

                <div className="row">
                    <div className="input-group">
                        <label>尺寸</label>
                        <select
                            value={formData.productSize}
                            onChange={(e) => setFormData({ ...formData, productSize: e.target.value })}
                            required
                        >
                            {PRODUCT_SIZES.map(size => (
                                <option key={size.id} value={size.id}>
                                    {size.value}
                                </option>
                            ))}
                        </select>
                    </div>
                    <div className="input-group">
                        <label>數量</label>
                        <input
                            type="number"
                            value={formData.quantity}
                            onChange={(e) => setFormData({ ...formData, quantity: e.target.value })}
                            onWheel={(e) => e.target.blur()}
                            min="1"
                            required
                        />
                    </div>
                </div>

                <div className="input-group">
                    <label>匯款帳號</label>
                    <div className="product-selection-header">
                        <select
                            value={formData.paymentAccountId}
                            onChange={(e) => setFormData({ ...formData, paymentAccountId: e.target.value })}
                            required
                        >
                            <option value="" disabled>請選擇匯款帳號</option>
                            {paymentAccountsList.map(acc => {
                                const last5 = acc.account_number ? String(acc.account_number).slice(-5) : '';
                                return (
                                    <option key={acc.id} value={acc.id}>
                                        {last5}-{acc.label}
                                    </option>
                                );
                            })}
                        </select>
                        <button
                            type="button"
                            className="refresh-btn"
                            onClick={() => fetchPaymentAccounts(true)}
                            disabled={loadingAccounts}
                            title="重新整理清單"
                        >
                            {loadingAccounts ? '...' : '刷新'}
                        </button>
                    </div>
                </div>

                <div className="input-group">
                    <label>產品描述 (選填)</label>
                    <textarea
                        value={formData.description}
                        onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                        rows="3"
                        className="admin-textarea"
                    />
                </div>
                <div className="row">
                    <div className="input-group">
                        <label>起標價</label>
                        <input
                            type="number"
                            value={formData.startPrice}
                            onChange={(e) => setFormData({ ...formData, startPrice: e.target.value })}
                            onWheel={(e) => e.target.blur()}
                            required
                        />
                    </div>
                    <div className="input-group">
                        <label>最小加價</label>
                        <input
                            type="number"
                            value={formData.minIncrement}
                            onChange={(e) => setFormData({ ...formData, minIncrement: e.target.value })}
                            onWheel={(e) => e.target.blur()}
                            required
                        />
                    </div>
                </div>
                <div className="row">
                    <div className="input-group">
                        <label>競標起始時間 (選填)</label>
                        <input
                            type="datetime-local"
                            value={formData.startTime}
                            onChange={(e) => setFormData({ ...formData, startTime: e.target.value })}
                        />
                    </div>
                    <div className="input-group">
                        <label>競標結束時間</label>
                        <input
                            type="datetime-local"
                            value={formData.endTime}
                            onChange={(e) => setFormData({ ...formData, endTime: e.target.value })}
                            required
                        />
                    </div>
                </div>
                <button type="submit" className="btn-primary w-full" disabled={submitting}>
                    {submitting ? '發佈中...' : '發佈產品'}
                </button>
            </form>

            <div className="admin-card glass-card verify-section">
                <h2>驗證使用者</h2>
                <UserVerification />
            </div>
        </div>
    );
};

const UserVerification = () => {
    const [userId, setUserId] = useState('');
    const [customerId, setCustomerId] = useState('');
    const [lineGroupName, setLineGroupName] = useState('');
    const [phone, setPhone] = useState('');
    const [loading, setLoading] = useState(false);

    // 使用者搜尋相關
    const [userSearchTerm, setUserSearchTerm] = useState('');
    const [userSearchResults, setUserSearchResults] = useState([]);
    const [showUserDropdown, setShowUserDropdown] = useState(false);
    const [searching, setSearching] = useState(false);
    const searchTimeoutRef = React.useRef(null);

    const searchUsers = async (term) => {
        if (!term || term.length < 1) {
            setUserSearchResults([]);
            setShowUserDropdown(false);
            return;
        }
        setSearching(true);
        try {
            const { data, error } = await supabase
                .from('profiles')
                .select('id, line_display_name, picture_url')
                .ilike('line_display_name', `%${term}%`)
                .limit(20);
            if (error) throw error;
            setUserSearchResults(data || []);
            setShowUserDropdown(true);
        } catch (err) {
            console.error('搜尋使用者失敗:', err);
        } finally {
            setSearching(false);
        }
    };

    const handleUserSearchChange = (e) => {
        const val = e.target.value;
        setUserSearchTerm(val);
        if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
        searchTimeoutRef.current = setTimeout(() => searchUsers(val), 300);
    };

    const handleUserSelect = (profile) => {
        setUserId(profile.id);
        setUserSearchTerm(profile.line_display_name || '');
        setShowUserDropdown(false);
    };

    // 點擊外部關閉下拉
    React.useEffect(() => {
        const handler = (e) => {
            if (!e.target.closest('.user-search-container')) {
                setShowUserDropdown(false);
            }
        };
        document.addEventListener('click', handler);
        return () => document.removeEventListener('click', handler);
    }, []);

    const handleVerify = async (e) => {
        e.preventDefault();
        setLoading(true);
        try {
            const { error } = await supabase.rpc('verify_user', {
                p_user_id: userId,
                p_customer_id: customerId,
                p_line_group_display_name: lineGroupName,
                p_phone: phone
            });
            if (error) throw error;
            alert('使用者驗證成功！');
            setUserId('');
            setCustomerId('');
            setLineGroupName('');
            setPhone('');
            setUserSearchTerm('');
        } catch (err) {
            alert('驗證失敗: ' + err.message);
        } finally {
            setLoading(false);
        }
    };

    return (
        <form onSubmit={handleVerify}>
            <div className="input-group">
                <label>搜尋使用者 (LINE 名稱)</label>
                <div className="user-search-container" style={{ position: 'relative' }}>
                    <input
                        type="text"
                        value={userSearchTerm}
                        onChange={handleUserSearchChange}
                        onFocus={() => { if (userSearchResults.length > 0) setShowUserDropdown(true); }}
                        placeholder="輸入 LINE 名稱搜尋..."
                    />
                    {searching && <div style={{ position: 'absolute', right: '10px', top: '50%', transform: 'translateY(-50%)', color: '#888', fontSize: '0.85rem' }}>搜尋中...</div>}
                    {showUserDropdown && userSearchResults.length > 0 && (
                        <div className="dropdown-list" style={{ maxHeight: '240px' }}>
                            {userSearchResults.map(profile => (
                                <div
                                    key={profile.id}
                                    className="dropdown-item"
                                    onClick={() => handleUserSelect(profile)}
                                    style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '8px 12px' }}
                                >
                                    <img
                                        src={profile.picture_url || 'https://via.placeholder.com/36'}
                                        alt=""
                                        style={{ width: '36px', height: '36px', borderRadius: '50%', objectFit: 'cover', flexShrink: 0 }}
                                        onError={(e) => { e.target.src = 'https://via.placeholder.com/36'; }}
                                    />
                                    <div style={{ overflow: 'hidden' }}>
                                        <div style={{ fontWeight: 'bold', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{profile.line_display_name || '(無名稱)'}</div>
                                        <div style={{ fontSize: '0.75rem', color: '#888', fontFamily: 'monospace' }}>{profile.id.slice(0, 8)}...</div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                    {showUserDropdown && userSearchResults.length === 0 && userSearchTerm.length >= 1 && !searching && (
                        <div className="dropdown-list">
                            <div className="dropdown-item no-results">找不到符合的使用者</div>
                        </div>
                    )}
                </div>
            </div>
            <div className="input-group">
                <label>使用者 ID (User ID)</label>
                <input
                    type="text"
                    value={userId}
                    onChange={(e) => setUserId(e.target.value)}
                    placeholder="請輸入使用者 UUID 或從上方搜尋選取"
                    required
                />
            </div>
            <div className="input-group">
                <label>鼠婦棲地使用者ID (選填)</label>
                <input
                    type="text"
                    value={customerId}
                    onChange={(e) => setCustomerId(e.target.value)}
                    placeholder="C00001"
                />
            </div>
            <div className="input-group">
                <label>LINE 群組顯示名稱</label>
                <input
                    type="text"
                    value={lineGroupName}
                    onChange={(e) => setLineGroupName(e.target.value)}
                    required
                />
            </div>
            <div className="input-group">
                <label>電話</label>
                <input
                    type="text"
                    value={phone}
                    onChange={(e) => setPhone(e.target.value)}
                    required
                />
            </div>
            <button type="submit" className="btn-primary w-full" disabled={loading}>
                {loading ? '處理中...' : '提交驗證'}
            </button>
        </form>
    );
};

export default Admin;
