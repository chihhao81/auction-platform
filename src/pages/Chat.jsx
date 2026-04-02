import React, { useState, useEffect, useCallback } from 'react';
import { useAuth } from '../context/AuthContext';
import { chatService } from '../utils/chatService';
import ChatRoomList from '../components/Chat/ChatRoomList';
import ChatMessageArea from '../components/Chat/ChatMessageArea';
import '../components/Chat/Chat.css';

const Chat = () => {
  const { user } = useAuth();
  const [rooms, setRooms] = useState([]);
  const [activeRoom, setActiveRoom] = useState(null);
  const [messages, setMessages] = useState([]);
  const [isMobile, setIsMobile] = useState(window.innerWidth <= 768);
  const [showRoomListOnMobile, setShowRoomListOnMobile] = useState(true);
  const [loading, setLoading] = useState(true);

  // Handle window resize for responsive layout
  useEffect(() => {
    const handleResize = () => setIsMobile(window.innerWidth <= 768);
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // Fetch rooms initially
  const fetchRooms = useCallback(async () => {
    try {
      setLoading(true);
      const data = await chatService.getChatRooms();
      setRooms(data || []);
    } catch (err) {
      console.error('Failed to fetch rooms', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (user) {
      fetchRooms();
    }
  }, [user, fetchRooms]);

  // Handle room selection
  const handleSelectRoom = async (room) => {
    setActiveRoom(room);
    if (isMobile) {
      setShowRoomListOnMobile(false);
    }

    try {
      const msgs = await chatService.getMessages(room.room_id);
      setMessages(msgs || []);
      
      // Mark as read
      await chatService.markAsRead(room.room_id, user.id);
      
      // Update local rooms state to clear unread badge
      setRooms(prev => prev.map(r => 
        r.room_id === room.room_id ? { ...r, has_unread: false } : r
      ));
    } catch (err) {
      console.error('Failed to fetch messages', err);
    }
  };

  // Realtime subscription setup
  useEffect(() => {
    if (!activeRoom) return;

    // We subscribe to the current active room
    const subscription = chatService.subscribeToMessages(activeRoom.room_id, (newMsg) => {
      // Append the new message
      setMessages(prev => [...prev, newMsg]);

      // If the message is not ours, mark it as read immediately if we are in the room
      if (newMsg.sender_id !== user.id) {
        chatService.markAsRead(activeRoom.room_id, user.id);
      }

      // Update room list preview text
      setRooms(prev => prev.map(r => 
        r.room_id === activeRoom.room_id 
          ? { ...r, last_message_content: newMsg.content || '[圖片]' }
          : r
      ));
    });

    return () => {
      subscription.unsubscribe();
    };
  }, [activeRoom, user.id]);

  const handleSendMessage = async (content, attachment) => {
    if (!activeRoom) return;
    
    // Send message using service
    await chatService.sendMessage(activeRoom.room_id, user.id, content, attachment);
    // Note: The message will be appended via the Realtime subscription, 
    // but sometimes for optimistic UI we could append it here. 
    // Supabase realtime provides it quite fast, so we'll wait for the broadcast.
  };

  const handleBackToList = () => {
    setShowRoomListOnMobile(true);
    setActiveRoom(null);
    fetchRooms(); // Refresh to get latest unread status if any
  };

  if (loading && rooms.length === 0) {
    return <div className="loading-screen">Loading Chat...</div>;
  }

  return (
    <div className="chat-container">
      <ChatRoomList 
        rooms={rooms} 
        activeRoomId={activeRoom?.room_id} 
        onSelectRoom={handleSelectRoom}
        isMobile={isMobile}
        showOnMobile={showRoomListOnMobile}
      />
      <ChatMessageArea 
        room={activeRoom} 
        messages={messages} 
        onSendMessage={handleSendMessage}
        isMobile={isMobile}
        showOnMobile={!showRoomListOnMobile}
        onBack={handleBackToList}
      />
    </div>
  );
};

export default Chat;
