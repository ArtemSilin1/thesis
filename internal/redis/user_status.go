package redis

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	onlineKey = "online:%s"      // online:{user_id}
	onlineTTL = 60 * time.Second // online status TTL
)

// SetOnline marks a user as online with expiration
func (c *Client) SetOnline(userID string) error {
	key := fmt.Sprintf(onlineKey, userID)

	return c.Set(c.ctx, key, time.Now().Unix(), onlineTTL).Err()
}

// SetOffline take off online status
func (c *Client) SetOffline(userID string) error {
	key := fmt.Sprintf(onlineKey, userID)

	return c.Del(c.ctx, key).Err()
}

// IsOnline checks if user online
func (c *Client) IsOnline(userID string) (bool, error) {
	key := fmt.Sprintf(onlineKey, userID)

	result, err := c.Exists(c.ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check online status: %w", err)
	}

	return result == 1, nil
}

// GetOnlineUsers gets online users list
func (c *Client) GetOnlineUsers(userIDs []string) (map[string]bool, error) {
	pipe := c.Pipeline()
	cmd := make(map[string]*redis.IntCmd)

	for _, userID := range userIDs {
		key := fmt.Sprintf(onlineKey, userID)
		cmd[userID] = pipe.Exists(c.ctx, key)
	}

	_, err := pipe.Exec(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get online users: %w", err)
	}

	result := make(map[string]bool)
	for userID, cmd := range cmd {
		val, _ := cmd.Result()
		result[userID] = val == 1
	}

	return result, nil
}

// RenewOnline keep online status
func (c *Client) RenewOnline(userID string) error {
	key := fmt.Sprintf(onlineKey, userID)

	return c.Expire(c.ctx, key, onlineTTL).Err()
}
