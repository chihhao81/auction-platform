import React from 'react';
import { useAuth } from '../../context/AuthContext';
const defaultImage = 'https://via.placeholder.com/48'; // Fallback

const ChatRoomList = ({ rooms, activeRoomId, onSelectRoom, isMobile, showOnMobile }) => {
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';

  if (isMobile && !showOnMobile) {
    return null;
  }

  return (
    <div className={`chat-sidebar ${isMobile && !showOnMobile ? 'hide-on-mobile' : ''}`}>
      <div className="chat-sidebar-header">
        訊息列表
      </div>
      <div className="chat-room-list">
        {rooms.length === 0 ? (
          <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            目前沒有對話
          </div>
        ) : (
          rooms.map(room => {
            const displayName = room.opponent_name || '使用者';
            const displayAvatar = room.opponent_avatar || defaultImage;

            return (
              <div 
                key={room.room_id} 
                className={`chat-room-item ${activeRoomId === room.room_id ? 'active' : ''}`}
                onClick={() => onSelectRoom(room)}
              >
                <img src={displayAvatar} alt="avatar" className="chat-room-avatar" />
                <div className="chat-room-info">
                  <div className="chat-room-name">{displayName}</div>
                  <div className="chat-room-preview">
                    {room.last_message_content || '暫無訊息'}
                  </div>
                </div>
                {room.has_unread && <div className="unread-badge"></div>}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};

export default ChatRoomList;
