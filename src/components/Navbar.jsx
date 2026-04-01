import React from 'react';
import { useAuth } from '../context/AuthContext';
import pkg from '../../package.json';
import { isTestEnv } from '../utils/envConfig';
import './Navbar.css';

const Navbar = () => {
    const { user, logout } = useAuth();
    const version = pkg.version;

    return (
        <nav className="navbar glass-card">
            <div className="nav-brand">
                <h1>
                    <span className="brand-title">特寵競標平台</span>
                    {isTestEnv && <span className="test-badge">測試台</span>}
                </h1>
                <span className="version">v{version}</span>
            </div>
            <div className="nav-actions">
                {user ? (
                    <>
                        <span className="user-email">
                            {user.line_group_name || user.line_name || '使用者'}
                        </span>
                        {user.isAdmin && <span className="badge-admin">Admin</span>}
                        <button onClick={logout} className="btn-logout">登出</button>
                    </>
                ) : (
                    <span className="nav-guest">歡迎來到競標平台</span>
                )}
            </div>
        </nav>
    );
};

export default Navbar;
