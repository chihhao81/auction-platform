import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.jsx'
import { AuthProvider } from './context/AuthContext'
import { AuctionProvider } from './context/AuctionContext'

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <AuthProvider>
      <AuctionProvider>
        <App />
      </AuctionProvider>
    </AuthProvider>
  </React.StrictMode>,
)
