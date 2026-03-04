package redis

import (
	"fmt"
)

const (
	chatMembersKey = "chat:%s:members" // chat:{chat_id}:members
)

// AddChatMember adding member to chat
func (c *Client) AddChatMember(chatID, userID string) error {
	key := fmt.Sprintf(chatMembersKey, chatID)

	return c.SAdd(c.ctx, key, userID).Err()
}

// RemoveChatMember deleting chat member
func (c *Client) RemoveChatMember(chatID, userID string) error {
	key := fmt.Sprintf(chatMembersKey, chatID)

	return c.SRem(c.ctx, key, userID).Err()
}

// GetChatMembers gets all chat members
func (c *Client) GetChatMembers(chatID string) ([]string, error) {
	key := fmt.Sprintf(chatMembersKey, chatID)

	result, err := c.SMembers(c.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get chat members: %w", err)
	}

	return result, nil
}

// GetOnlineChatMembers gets online statuses of users
func (c *Client) GetOnlineChatMembers(chatID string) ([]string, error) {
	members, err := c.GetChatMembers(chatID)
	if err != nil {
		return nil, err
	}

	online := make([]string, 0)
	for _, userID := range members {
		isOnline, _ := c.IsOnline(userID)
		if isOnline {
			online = append(online, userID)
		}
	}

	return online, nil
}
