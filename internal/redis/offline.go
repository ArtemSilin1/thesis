package redis

import (
	"fmt"
	"time"
)

const (
	offlineTTL = 7 * 24 * time.Hour
)

// AddOfflineMessage add a message to the user's offline list
func (c *Client) AddOfflineMessage(userID string, message *Message) error {
	key := fmt.Sprintf(offlineKey, userID)

	msgID := message.ID
	if msgID == "" {
		msgID = message.ChatID + ":" + time.Now().String()
	}

	_, err := c.LPush(c.ctx, key, msgID).Result()
	if err != nil {
		return fmt.Errorf("failed to add offline message: %w", err)
	}

	c.Expire(c.ctx, key, offlineTTL)

	return nil
}

// GetOfflineMessages gets all offline messages
func (c *Client) GetOfflineMessages(userID string) ([]string, error) {
	key := fmt.Sprintf(offlineKey, userID)

	result, err := c.LRange(c.ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get offline messages: %w", err)
	}

	c.Del(c.ctx, key)

	return result, nil
}

// HasOfflineMessages checks if offline messages exist
func (c *Client) HasOfflineMessages(userID string) (bool, error) {
	key := fmt.Sprintf(offlineKey, userID)

	result, err := c.LLen(c.ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check offline messages: %w", err)
	}

	return result > 0, nil
}
