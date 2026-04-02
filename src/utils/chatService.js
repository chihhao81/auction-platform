import { supabase } from './supabaseClient';

export const chatService = {
  /**
   * 取得使用者的有效聊天室列表
   */
  async getChatRooms() {
    const { data, error } = await supabase
      .from('active_chat_sessions')
      .select('*')
      .order('room_created_at', { ascending: false });

    if (error) {
      console.error('Error fetching chat rooms:', error);
      throw error;
    }
    return data;
  },

  /**
   * 取得特定聊天室的所有訊息
   */
  async getMessages(roomId) {
    const { data, error } = await supabase
      .from('chat_messages')
      .select('*, sender:profiles!chat_messages_sender_id_fkey(line_group_display_name, picture_url)')
      .eq('room_id', roomId)
      .order('created_at', { ascending: true });

    if (error) {
      console.error('Error fetching messages:', error);
      throw error;
    }
    return data;
  },

  /**
   * 傳送文字訊息或圖片
   */
  async sendMessage(roomId, senderId, content, attachmentFile = null) {
    let attachmentUrl = null;

    if (attachmentFile) {
      // 產生唯一檔名避免覆蓋
      const fileExt = attachmentFile.name.split('.').pop();
      const fileName = `${Date.now()}_${Math.random().toString(36).substring(7)}.${fileExt}`;
      const filePath = `${roomId}/${fileName}`;

      const { data: uploadData, error: uploadError } = await supabase.storage
        .from('chat_images')
        .upload(filePath, attachmentFile);

      if (uploadError) {
        console.error('Error uploading image:', uploadError);
        throw uploadError;
      }
      
      const { data: { publicUrl } } = supabase.storage
        .from('chat_images')
        .getPublicUrl(filePath);
        
      attachmentUrl = publicUrl;
    }

    const { data, error } = await supabase
      .from('chat_messages')
      .insert([
        {
          room_id: roomId,
          sender_id: senderId,
          content: content,
          attachment_url: attachmentUrl
        }
      ])
      .select()
      .single();

    if (error) {
      console.error('Error sending message:', error);
      throw error;
    }
    
    // 更新 chat_rooms last_message_at
    await supabase.from('chat_rooms')
      .update({ last_message_at: new Date().toISOString() })
      .eq('id', roomId);

    return data;
  },

  /**
   * 標記該聊天室的他人訊息為已讀
   */
  async markAsRead(roomId, currentUserId) {
    const { error } = await supabase
      .from('chat_messages')
      .update({ read_at: new Date().toISOString() })
      .eq('room_id', roomId)
      .neq('sender_id', currentUserId)
      .is('read_at', null);

    if (error) {
      console.error('Error marking messages as read:', error);
    }
  },

  /**
   * 訂閱新訊息 Realtime，回傳 subscription object 以便清理
   */
  subscribeToMessages(roomId, onMessageCallback) {
    return supabase
      .channel(`chat_room_${roomId}`)
      .on(
        'postgres_changes',
        { event: 'INSERT', schema: 'public', table: 'chat_messages', filter: `room_id=eq.${roomId}` },
        payload => {
          onMessageCallback(payload.new);
        }
      )
      .subscribe();
  }
};
