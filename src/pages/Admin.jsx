import React, { useState, useEffect, useRef } from 'react';
import { useAuction } from '../context/AuctionContext';
import { supabase } from '../utils/supabaseClient';
import './Admin.css';

const Admin = () => {
    return (
        <div className="admin-container">
            <AuctionManager />
            <UserSearch />
        </div>
    );
};

const AuctionManager = () => {
    const { fetchAuctions, auctions } = useAuction();
    const [submitting, setSubmitting] = useState(false);
    const fileInputRef = useRef(null);

    // Form States
    const [editMode, setEditMode] = useState(false);
    const [editingId, setEditingId] = useState(null);

    // existingImages: URLs from DB that we keep; imageFiles: new File objects to upload
    const [existingImages, setExistingImages] = useState([]);

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
        title: '',
        description: localStorage.getItem('aura_last_description') || '',
        startPrice: '0',
        minIncrement: '50',
        startTime: getCurrentTime(),
        endTime: getDefaultEndTime(),
    });
    const [imageFiles, setImageFiles] = useState([]);

    // Listen for edit requests from AuctionCard three-dot menu
    useEffect(() => {
        const handleEditEvent = (e) => {
            handleEdit(e.detail);
        };
        window.addEventListener('admin-edit-auction', handleEditEvent);
        return () => window.removeEventListener('admin-edit-auction', handleEditEvent);
    }, []);

    const handleAddImages = (e) => {
        const newFiles = Array.from(e.target.files);
        if (newFiles.length > 0) {
            setImageFiles(prev => [...prev, ...newFiles]);
        }
        // 重設 input 讓同一檔案可以再選
        e.target.value = '';
    };

    const removeImage = (index) => {
        setImageFiles(prev => prev.filter((_, i) => i !== index));
    };

    const uploadImages = async (files) => {
        const uploadedUrls = [];
        for (const file of files) {
            const fileExt = file.name.split('.').pop();
            const fileName = `${Date.now()}_${Math.random().toString(36).substring(7)}.${fileExt}`;
            const { data, error } = await supabase.storage
                .from('product_images')
                .upload(fileName, file);
            if (error) {
                console.error("Upload error:", error);
                throw new Error("圖片上傳失敗");
            }
            const { data: publicUrlData } = supabase.storage
                .from('product_images')
                .getPublicUrl(fileName);
            uploadedUrls.push(publicUrlData.publicUrl);
        }
        return uploadedUrls;
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!formData.title) return alert('請輸入標題');
        if (!editMode && imageFiles.length === 0) return alert('請至少上傳一張圖片');
        if (editMode && existingImages.length === 0 && imageFiles.length === 0) return alert('請至少保留或上傳一張圖片');

        setSubmitting(true);
        try {
            const finalStartTime = formData.startTime ? new Date(formData.startTime) : new Date();
            const finalEndTime = new Date(formData.endTime);

            localStorage.setItem('aura_last_description', formData.description || '');

            let imageUrls = [];

            // Combine existing images (URLs kept from DB) + newly uploaded files
            if (editMode) {
                imageUrls = [...existingImages];
            }
            if (imageFiles.length > 0) {
                const newUrls = await uploadImages(imageFiles);
                imageUrls = [...imageUrls, ...newUrls];
            }
            if (imageUrls.length === 0 && !editMode) {
                return alert('請至少上傳一張圖片');
            }

            if (editMode) {
                const { error } = await supabase.rpc('update_auction', {
                    p_auction_id: editingId,
                    p_description: formData.description || '',
                    p_end_time: finalEndTime.toISOString(),
                    p_images: imageUrls,
                    p_min_increment: Number(formData.minIncrement),
                    p_start_price: Number(formData.startPrice),
                    p_start_time: finalStartTime.toISOString(),
                    p_title: formData.title
                });
                if (error) throw error;
                alert('編輯成功！');
            } else {
                const { error } = await supabase.rpc('create_auction', {
                    p_description: formData.description || '',
                    p_end_time: finalEndTime.toISOString(),
                    p_images: imageUrls,
                    p_min_increment: Number(formData.minIncrement),
                    p_start_price: Number(formData.startPrice),
                    p_start_time: finalStartTime.toISOString(),
                    p_title: formData.title
                });
                if (error) throw error;
                alert('發佈成功！');
            }

            resetForm();
            fetchAuctions();
        } catch (error) {
            alert((editMode ? '編輯失敗: ' : '發佈失敗: ') + error.message);
        } finally {
            setSubmitting(false);
        }
    };

    const resetForm = () => {
        setFormData({
            title: '',
            description: localStorage.getItem('aura_last_description') || '',
            startPrice: '0',
            minIncrement: '50',
            startTime: getCurrentTime(),
            endTime: getDefaultEndTime(),
        });
        setImageFiles([]);
        setExistingImages([]);
        setEditMode(false);
        setEditingId(null);
    };

    const handleEdit = (auction) => {
        setEditMode(true);
        setEditingId(auction.id);

        const formatForInput = (dateObj) => {
            const pad = (n) => String(n).padStart(2, '0');
            return `${dateObj.getFullYear()}-${pad(dateObj.getMonth() + 1)}-${pad(dateObj.getDate())}T${pad(dateObj.getHours())}:${pad(dateObj.getMinutes())}`;
        };

        setFormData({
            title: auction.name,
            description: auction.description || '',
            startPrice: auction.startPrice,
            minIncrement: auction.minIncrement,
            startTime: formatForInput(new Date(auction.startTime)),
            endTime: formatForInput(new Date(auction.endTime)),
        });
        // Load existing images as URLs
        setExistingImages(auction.images || []);
        setImageFiles([]);
        window.scrollTo({ top: 0, behavior: 'smooth' });
    };



    return (
            <form className="admin-card glass-card" onSubmit={handleSubmit}>
                <h2>{editMode ? '編輯競標產品' : '新增競標產品'}</h2>
                {editMode && (
                    <div style={{ marginBottom: '1rem' }}>
                        <button type="button" className="btn-secondary" onClick={resetForm}>取消編輯</button>
                    </div>
                )}

                <div className="input-group">
                    <label>競標標題</label>
                    <input
                        type="text"
                        value={formData.title}
                        onChange={e => setFormData({ ...formData, title: e.target.value })}
                        required
                    />
                </div>

                <div className="input-group">
                    <label>圖片</label>
                    <input
                        ref={fileInputRef}
                        type="file"
                        multiple
                        accept="image/*"
                        onChange={handleAddImages}
                        style={{ display: 'none' }}
                    />
                    <button
                        type="button"
                        className="image-upload-btn"
                        onClick={() => fileInputRef.current?.click()}
                    >
                        📷 選擇圖片
                    </button>
                    {(existingImages.length > 0 || imageFiles.length > 0) && (
                        <div className="image-preview-list">
                            {existingImages.map((url, i) => (
                                <div key={`existing-${i}`} className="image-preview-item">
                                    <img src={url} alt={`existing ${i}`} />
                                    <button type="button" className="image-remove-btn" onClick={() => setExistingImages(prev => prev.filter((_, idx) => idx !== i))}>&times;</button>
                                </div>
                            ))}
                            {imageFiles.map((file, i) => (
                                <div key={`new-${i}`} className="image-preview-item">
                                    <img src={URL.createObjectURL(file)} alt={file.name} />
                                    <button type="button" className="image-remove-btn" onClick={() => removeImage(i)}>&times;</button>
                                    <span className="image-preview-name">{file.name}</span>
                                </div>
                            ))}
                        </div>
                    )}
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
                    {submitting ? '處理中...' : (editMode ? '儲存變更' : '發佈產品')}
                </button>
            </form>
    );
};

const UserSearch = () => {
    const [searchTerm, setSearchTerm] = useState('');
    const [searchResults, setSearchResults] = useState([]);
    const [searching, setSearching] = useState(false);

    const handleSearch = async (e) => {
        e.preventDefault();
        if (!searchTerm) return;
        setSearching(true);
        try {
            const { data, error } = await supabase
                .from('profiles')
                .select('phone, line_display_name, line_group_display_name, picture_url')
                .ilike('line_group_display_name', `%${searchTerm}%`)
                .limit(20);

            if (error) throw error;
            setSearchResults(data || []);
        } catch (err) {
            console.error('User search error:', err);
            alert('搜尋失敗: ' + err.message);
        } finally {
            setSearching(false);
        }
    };

    return (
        <div className="admin-card glass-card">
            <h2>使用者查詢</h2>
            <form onSubmit={handleSearch}>
                <div className="input-group">
                    <label>輸入 LINE 群組顯示名稱</label>
                    <div style={{ display: 'flex', gap: '10px' }}>
                        <input
                            type="text"
                            style={{ flex: 1 }}
                            value={searchTerm}
                            onChange={e => setSearchTerm(e.target.value)}
                            placeholder="支援模糊搜尋..."
                        />
                        <button type="submit" className="btn-primary" disabled={searching}>搜尋</button>
                    </div>
                </div>
            </form>

            {searchResults.length > 0 && (
                <div style={{ marginTop: '1rem', display: 'flex', flexDirection: 'column', gap: '10px' }}>
                    {searchResults.map((user, idx) => (
                        <div key={idx} style={{ padding: '10px', background: 'rgba(255,255,255,0.05)', borderRadius: '8px', display: 'flex', alignItems: 'center', gap: '15px' }}>
                            <img src={user.picture_url || 'https://via.placeholder.com/40'} alt="avatar" style={{ width: '40px', height: '40px', borderRadius: '50%', objectFit: 'cover' }} />
                            <div>
                                <div style={{ fontWeight: 'bold', color: '#fff' }}>{user.line_group_display_name || '無群組名稱'}</div>
                                <div style={{ fontSize: '0.85rem', color: '#ccc' }}>LINE 名稱：{user.line_display_name || '無'}</div>
                                <div style={{ fontSize: '0.85rem', color: '#ccc' }}>電話：{user.phone || '無'}</div>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            {!searching && searchTerm && searchResults.length === 0 && (
                <p style={{ marginTop: '1rem', color: '#aaa' }}>找不到符合條件的使用者。</p>
            )}
        </div>
    );
};

export default Admin;
