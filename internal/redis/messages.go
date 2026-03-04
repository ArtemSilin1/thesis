package redis

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis keys
	messagesKey = "chat:%s:messages" // chat:{chat_id}:messages
	unreadKey   = "unread:%s"        // unread:{user_id}
	offlineKey  = "offline:%s"       // offline:{user_id}

	// Settings
	maxMessagesPerChat = 1000               // store last 1000 messages per chat
	messageTTL         = 7 * 24 * time.Hour // 7 days in Redis
)

// SaveMessage saves a message to Redis
func (c *Client) SaveMessage(msg *Message) error {
	key := fmt.Sprintf(messagesKey, msg.ChatID)

	score := float64(msg.CreatedAt.UnixNano()) / 1e9

	if err := c.ZAdd(c.ctx, key, redis.Z{
		Score:  score,
		Member: msg,
	}).Err(); err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	c.Expire(c.ctx, key, messageTTL)

	c.ZRemRangeByRank(c.ctx, key, 0, -maxMessagesPerChat-1)

	return nil
}

// GetRecentMessages retrieves the most recent messages from a chat
func (c *Client) GetRecentMessages(chatID string, limit int64) ([]*Message, error) {
	key := fmt.Sprintf(messagesKey, chatID)

	// Get the last limit messages
	result, err := c.ZRange(c.ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	messages := make([]*Message, 0, len(result))
	for _, item := range result {
		var msg Message
		if err := json.Unmarshal([]byte(item), &msg); err != nil {
			continue // skip corrupted messages
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// GetMessagesBefore retrieves messages before a specific time (for pagination)
func (c *Client) GetMessagesBefore(chatID string, before time.Time, limit int64) ([]*Message, error) {
	key := fmt.Sprintf(messagesKey, chatID)

	score := float64(before.UnixNano()) / 1e9

	result, err := c.ZRevRangeByScoreWithScores(c.ctx, key, &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%f", score),
		Offset: 0,
		Count:  limit,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	messages := make([]*Message, 0, len(result))
	for _, z := range result {
		var msg Message
		if err := json.Unmarshal([]byte(z.Member.(string)), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// IncrementUnread increments the unread counter for a user
func (c *Client) IncrementUnread(userID, chatID string) error {
	key := fmt.Sprintf(unreadKey, userID)

	// Increment counter for a specific chat
	_, err := c.HIncrBy(c.ctx, key, chatID, 1).Result()
	if err != nil {
		return fmt.Errorf("failed to increment unread: %w", err)
	}

	return nil
}

// GetUnreadCounts gets all unread counts for a user
func (c *Client) GetUnreadCounts(userID string) (map[string]int64, error) {
	key := fmt.Sprintf(unreadKey, userID)

	result, err := c.HGetAll(c.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get unread counts: %w", err)
	}

	counts := make(map[string]int64)
	for chatID, countStr := range result {
		var count int64
		fmt.Sscanf(countStr, "%d", &count)
		counts[chatID] = count
	}

	return counts, nil
}

// MarkAsRead marks messages as read
func (c *Client) MarkAsRead(userID, chatID string, messageID string) error {
	key := fmt.Sprintf(unreadKey, userID)

	// Delete counter for this chat
	_, err := c.HDel(c.ctx, key, chatID).Result()
	if err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	c.Set(c.ctx, fmt.Sprintf("read:%s:%s", userID, chatID), messageID, 0)

	return nil
}
