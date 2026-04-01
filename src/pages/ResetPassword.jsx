import React, { useState } from 'react';
import { supabase } from '../utils/supabaseClient';
import { useNavigate } from 'react-router-dom';
import './Login.css'; // Reuse login styles

const ResetPassword = () => {
    const [password, setPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [loading, setLoading] = useState(false);
    const [errorMsg, setErrorMsg] = useState('');
    const navigate = useNavigate();

    const handleUpdatePassword = async (e) => {
        e.preventDefault();
        setErrorMsg('');

        if (password.length < 6) {
            setErrorMsg('密碼長度必須至少為 6 位數');
            return;
        }

        if (password !== confirmPassword) {
            setErrorMsg('兩次輸入的密碼不一致');
            return;
        }

        setLoading(true);
        try {
            const { error } = await supabase.auth.updateUser({
                password: password
            });
            if (error) throw error;
            await supabase.auth.signOut();
            alert('密碼重設成功，請使用新密碼重新登入');
            navigate('/');
        } catch (err) {
            setErrorMsg(err.message || '重設失敗，請稍後再試');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="login-container">
            <div className="login-card glass-card">
                <form onSubmit={handleUpdatePassword}>
                    <h2>重設密碼</h2>
                    <p>請輸入您的新密碼</p>

                    <div className="input-group">
                        <label>新密碼</label>
                        <input
                            type="password"
                            placeholder="請輸入新密碼"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            required
                        />
                    </div>

                    <div className="input-group">
                        <label>確認新密碼</label>
                        <input
                            type="password"
                            placeholder="請再次輸入新密碼"
                            value={confirmPassword}
                            onChange={(e) => setConfirmPassword(e.target.value)}
                            required
                        />
                    </div>

                    {errorMsg && <p className="error-message">{errorMsg}</p>}

                    <button type="submit" className="btn-primary w-full" disabled={loading}>
                        {loading ? '正在更新...' : '更新密碼'}
                    </button>
                </form>
            </div>
        </div>
    );
};

export default ResetPassword;
