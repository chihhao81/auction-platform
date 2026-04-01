import React from 'react';
import { useNavigate } from 'react-router-dom';
import './Login.css';

const LoginError = () => {
    const navigate = useNavigate();
    const params = new URLSearchParams(window.location.search);
    const errorMsg = params.get('message') || '登入過程中發生未知錯誤';

    return (
        <div className="login-container">
            <div className="login-card glass-card" style={{ textAlign: 'center' }}>
                <div className="login-error-icon">✕</div>
                <h2>登入失敗</h2>
                <p className="error-message">{errorMsg}</p>
                <button
                    className="btn-primary w-full"
                    onClick={() => navigate('/')}
                >
                    返回登入頁
                </button>
            </div>
        </div>
    );
};

export default LoginError;
