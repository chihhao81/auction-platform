import React, { useState, useRef, useEffect } from 'react';
import { useAuth } from '../../context/AuthContext';

const defaultImage = 'https://via.placeholder.com/48';

const ChatMessageArea = ({ room, messages, onSendMessage, isMobile, showOnMobile, onBack }) => {
  const { user } = useAuth();
  const [inputText, setInputText] = useState('');
  const [attachment, setAttachment] = useState(null);
  const [isSending, setIsSending] = useState(false);
  const messagesEndRef = useRef(null);
  const fileInputRef = useRef(null);

  // Scroll to bottom when messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  if (isMobile && !showOnMobile) {
    return null;
  }

  if (!room) {
    return (
      <div className={`chat-main ${isMobile && !showOnMobile ? 'hide-on-mobile' : ''} chat-empty-state`}>
        請選擇一個對話開始聊天
      </div>
    );
  }

  const displayName = room.opponent_name || '使用者';
  const displayAvatar = room.opponent_avatar || defaultImage;

  const handleSend = async () => {
    if (!inputText.trim() && !attachment) return;

    setIsSending(true);
    try {
      await onSendMessage(inputText, attachment);
      setInputText('');
      setAttachment(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
    } catch (err) {
      alert('傳送失敗，請稍後再試');
    } finally {
      setIsSending(false);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleFileChange = (e) => {
    if (e.target.files && e.target.files[0]) {
      setAttachment(e.target.files[0]);
    }
  };

  return (
    <div className={`chat-main ${isMobile && !showOnMobile ? 'hide-on-mobile' : ''}`}>
      <div className="chat-main-header">
        {isMobile && (
          <button className="back-btn" onClick={onBack}>
            ←
          </button>
        )}
        <img src={displayAvatar} alt="avatar" className="chat-room-avatar" style={{width: 36, height: 36}} />
        <span style={{ fontWeight: 'bold' }}>{displayName}</span>
      </div>

      <div className="chat-messages">
        {messages.map((msg) => {
          const isMine = msg.sender_id === user.id;
          const msgAvatar = isMine 
            ? (user.picture_url || defaultImage) 
            : displayAvatar;

          return (
            <div key={msg.id} className={`message-wrapper ${isMine ? 'mine' : 'theirs'}`}>
              <img src={msgAvatar} alt="avatar" className="message-avatar" />
              <div className="message-content">
                <div className="message-bubble">
                  {msg.content}
                  {msg.attachment_url && (
                    <img 
                      src={msg.attachment_url} 
                      alt="附件" 
                      className="message-attachment" 
                      onClick={() => window.open(msg.attachment_url, '_blank')}
                    />
                  )}
                </div>
                <div className="message-time">
                  {new Date(msg.created_at).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' })}
                </div>
              </div>
            </div>
          );
        })}
        <div ref={messagesEndRef} />
      </div>

      <div className="chat-input-area">
        <input 
          type="file" 
          ref={fileInputRef}
          style={{ display: 'none' }} 
          accept="image/*"
          onChange={handleFileChange}
        />
        <button 
          className="chat-attachment-btn"
          onClick={() => fileInputRef.current?.click()}
          title="上傳圖片"
        >
          {attachment ? '🖼️ (已選)' : '📎'}
        </button>
        <textarea 
          className="chat-input"
          placeholder="輸入訊息..."
          value={inputText}
          onChange={(e) => setInputText(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={1}
          disabled={isSending}
        />
        <button 
          className="chat-send-btn"
          onClick={handleSend}
          disabled={isSending || (!inputText.trim() && !attachment)}
        >
          {isSending ? '...' : '➤'}
        </button>
      </div>
    </div>
  );
};

export default ChatMessageArea;
