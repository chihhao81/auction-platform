import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Navbar from './components/Navbar';
import Footer from './components/Footer';
import Home from './pages/Home';
import Login from './pages/Login';
import LoginError from './pages/LoginError';
import Admin from './pages/Admin';
import ResetPassword from './pages/ResetPassword';
import { useAuth } from './context/AuthContext';
import './styles/global.css';

function App() {
  const { user, loading } = useAuth();

  if (loading) return <div className="loading-screen">Loading...</div>;

  return (
    <BrowserRouter basename={import.meta.env.BASE_URL}>
      <div className="app">
        <Navbar />
        <main>
          <Routes>
            <Route path="/" element={
              !user ? (
                <Login />
              ) : (
                <>
                  {user.role === 'admin' && <Admin />}
                  <Home />
                </>
              )
            } />
            <Route path="/reset-password" element={<ResetPassword />} />
            <Route path="/login-error" element={<LoginError />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
        {user && <Footer />}
      </div>
    </BrowserRouter>
  );
}

export default App;
