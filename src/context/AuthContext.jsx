import React, { createContext, useContext, useState, useEffect } from 'react';
import { supabase } from '../utils/supabaseClient';

const AuthContext = createContext();

export const AuthProvider = ({ children }) => {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);
    const lastFetchedId = React.useRef(null);

    useEffect(() => {
        const getProfile = async (session) => {
            if (!session) {
                setUser(null);
                setLoading(false);
                lastFetchedId.current = null;
                return;
            }

            // 如果已經在抓取或抓過同一個人的了，確保 loading 結束後直接返回
            if (lastFetchedId.current === session.user.id) {
                // 如果已經有 user 了，代表已經完成
                if (user) setLoading(false);
                return;
            }
            lastFetchedId.current = session.user.id;

            try {
                const { data, error } = await supabase
                    .from('profiles')
                    .select('*')
                    .eq('id', session.user.id)
                    .single();

                if (error) {
                    console.error('Error fetching profile:', error);
                    setUser({ ...session.user, isAdmin: false });
                } else if (data.is_banned) {
                    alert('您的帳號已被停權，請聯繫Line官方帳號。');
                    await supabase.auth.signOut();
                    setUser(null);
                } else {
                    setUser({
                        ...session.user,
                        role: data.role,
                        is_verified: data.is_verified,
                        is_banned: data.is_banned,
                        line_name: data.line_display_name,
                        line_group_name: data.line_group_display_name,
                        isAdmin: data.role === 'admin'
                    });
                }
            } catch (err) {
                console.error('Auth error:', err);
                setUser({ ...session.user, isAdmin: false });
            } finally {
                setLoading(false);
            }
        };

        // 確保初始載入能抓到 session
        supabase.auth.getSession().then(({ data: { session } }) => {
            getProfile(session);
        });

        // Listen for auth changes
        const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
            getProfile(session);
        });

        return () => subscription.unsubscribe();
    }, []);

    const login = async (email, password) => {
        const { error } = await supabase.auth.signInWithPassword({ email, password });
        if (error) throw error;
    };

    const signup = async (email, password) => {
        const { error } = await supabase.auth.signUp({
            email,
            password,
            options: {
                emailRedirectTo: window.location.origin + import.meta.env.BASE_URL,
            },
        });
        if (error) throw error;
    };

    const logout = async () => {
        await supabase.auth.signOut();
    };

    const resetPassword = async (email) => {
        const basePath = import.meta.env.BASE_URL;
        const { error } = await supabase.auth.resetPasswordForEmail(email, {
            redirectTo: window.location.origin + basePath + (basePath.endsWith('/') ? '' : '/') + 'reset-password',
        });
        if (error) throw error;
    };

    return (
        <AuthContext.Provider value={{ user, login, logout, signup, resetPassword, loading, isAdmin: user?.isAdmin }}>
            {!loading && children}
        </AuthContext.Provider>
    );
};

export const useAuth = () => useContext(AuthContext);
