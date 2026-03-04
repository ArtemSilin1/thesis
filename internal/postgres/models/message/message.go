package message

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	cache "thesis/internal/redis"
)

// Common errors
var (
	ErrMessageNotFound = errors.New("message not found")
	ErrChatNotFound    = errors.New("chat not found")
	ErrUserNotInChat   = errors.New("user is not a member of this chat")
	ErrInvalidMessage  = errors.New("invalid message data")
)

// MessageType defines possible message types
type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeFile  MessageType = "file"
	MessageTypeImage MessageType = "image"
	MessageTypeVideo MessageType = "video"
	MessageTypeAudio MessageType = "audio"
)

// Message represents a message in the system
type Message struct {
	ID        string      `json:"id"`
	ChatID    string      `json:"chat_id"`
	SenderID  string      `json:"sender_id"`
	Content   string      `json:"content"`
	Type      MessageType `json:"type"`
	FileURL   *string     `json:"file_url,omitempty"`
	ReplyTo   *string     `json:"reply_to,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// MessageArchive represents the database model for message_archive table
type MessageArchive struct {
	ID        int64     `db:"id"`
	ChatID    uuid.UUID `db:"chat_id"`
	MessageID int64     `db:"message_id"`
	SenderID  uuid.UUID `db:"sender_id"`
	Content   string    `db:"content"`
	Type      string    `db:"type"`
	FileURL   *string   `db:"file_url"`
	CreatedAt time.Time `db:"created_at"`
}

// MessageFilter defines filtering options for message queries
type MessageFilter struct {
	Limit      int
	Offset     int64
	BeforeTime *time.Time
	AfterTime  *time.Time
	SenderID   *string
	MessageIDs []string
}

// Service handles message business logic
type Service struct {
	db          *sql.DB
	redisClient *redis.Client
	cacheSvc    *cache.Client
}

// NewService creates a new message service instance
func NewService(db *sql.DB, redisClient *redis.Client, cacheSvc *cache.Client) *Service {
	return &Service{
		db:          db,
		redisClient: redisClient,
		cacheSvc:    cacheSvc,
	}
}

// SendMessage handles sending a new message
func (s *Service) SendMessage(ctx context.Context, msg *Message) error {
	// Validate message
	if msg == nil {
		return ErrInvalidMessage
	}
	if msg.ChatID == "" || msg.SenderID == "" || msg.Content == "" {
		return ErrInvalidMessage
	}

	// Start a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Verify user is a member of the chat
	var isMember bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chat_members 
			WHERE chat_id = $1 AND user_id = $2
		)
	`, msg.ChatID, msg.SenderID).Scan(&isMember)
	if err != nil {
		return fmt.Errorf("failed to verify chat membership: %w", err)
	}
	if !isMember {
		return ErrUserNotInChat
	}

	// 2. Insert message into archive
	messageID := generateMessageID()
	var archiveID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO message_archive (chat_id, message_id, sender_id, content, type, file_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, msg.ChatID, messageID, msg.SenderID, msg.Content, string(msg.Type), msg.FileURL, msg.CreatedAt).Scan(&archiveID)
	if err != nil {
		return fmt.Errorf("failed to insert message into archive: %w", err)
	}

	// Set the generated message ID
	msg.ID = fmt.Sprintf("%d", messageID)

	// 3. Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 4. Save to Redis for fast access
	redisMsg := &cache.Message{
		ID:        msg.ID,
		ChatID:    msg.ChatID,
		SenderID:  msg.SenderID,
		Content:   msg.Content,
		Type:      string(msg.Type),
		CreatedAt: msg.CreatedAt,
	}

	// Set optional fields
	if msg.FileURL != nil {
		redisMsg.FileURL = *msg.FileURL
	}
	if msg.ReplyTo != nil {
		redisMsg.ReplyTo = *msg.ReplyTo
	}

	if err := s.cacheSvc.SaveMessage(redisMsg); err != nil {
		// Log error but don't fail the operation - Redis is for caching
		fmt.Printf("Failed to save message to Redis: %v\n", err)
	}

	// 5. Update unread counts for all chat members except sender
	if err := s.updateUnreadCounts(ctx, msg.ChatID, msg.SenderID); err != nil {
		fmt.Printf("Failed to update unread counts: %v\n", err)
	}

	return nil
}

// GetMessages retrieves messages from a chat with pagination
func (s *Service) GetMessages(ctx context.Context, chatID, userID string, filter MessageFilter) ([]*Message, error) {
	// Verify user has access to this chat
	hasAccess, err := s.verifyChatAccess(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, ErrUserNotInChat
	}

	// Try to get from Redis first (for recent messages)
	if filter.BeforeTime == nil && filter.AfterTime == nil && filter.Limit > 0 && filter.Limit <= 50 {
		messages, err := s.getMessagesFromRedis(ctx, chatID, int64(filter.Limit))
		if err == nil && len(messages) > 0 {
			return messages, nil
		}
	}

	// If not in Redis or need older messages, get from PostgreSQL
	return s.getMessagesFromDB(ctx, chatID, filter)
}

// GetMessageByID retrieves a single message by ID
func (s *Service) GetMessageByID(ctx context.Context, messageID, userID string) (*Message, error) {
	// Parse message ID
	var msgID int64
	_, err := fmt.Sscanf(messageID, "%d", &msgID)
	if err != nil {
		return nil, ErrInvalidMessage
	}

	// Get from DB
	var archive MessageArchive
	err = s.db.QueryRowContext(ctx, `
		SELECT id, chat_id, message_id, sender_id, content, type, file_url, created_at
		FROM message_archive
		WHERE message_id = $1
	`, msgID).Scan(
		&archive.ID, &archive.ChatID, &archive.MessageID, &archive.SenderID,
		&archive.Content, &archive.Type, &archive.FileURL, &archive.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	// Verify user has access to this chat
	hasAccess, err := s.verifyChatAccess(ctx, archive.ChatID.String(), userID)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, ErrUserNotInChat
	}

	return s.convertArchiveToMessage(&archive), nil
}

// DeleteMessage deletes a message
func (s *Service) DeleteMessage(ctx context.Context, messageID, userID string) error {
	// Parse message ID
	var msgID int64
	_, err := fmt.Sscanf(messageID, "%d", &msgID)
	if err != nil {
		return ErrInvalidMessage
	}

	// Check if message exists and user is the sender
	var senderID uuid.UUID
	var chatID uuid.UUID
	err = s.db.QueryRowContext(ctx, `
		SELECT sender_id, chat_id FROM message_archive WHERE message_id = $1
	`, msgID).Scan(&senderID, &chatID)
	if err == sql.ErrNoRows {
		return ErrMessageNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}

	// Verify user is the sender
	if senderID.String() != userID {
		return errors.New("user is not the sender of this message")
	}

	// Delete from database
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM message_archive WHERE message_id = $1
	`, msgID)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

// MarkMessagesAsRead marks messages as read for a user
func (s *Service) MarkMessagesAsRead(ctx context.Context, chatID, userID string, lastReadMessageID *string) error {
	// Verify user is in chat
	hasAccess, err := s.verifyChatAccess(ctx, chatID, userID)
	if err != nil {
		return err
	}
	if !hasAccess {
		return ErrUserNotInChat
	}

	// Parse last read message ID if provided
	var lastReadID *int64
	if lastReadMessageID != nil && *lastReadMessageID != "" {
		var id int64
		_, err := fmt.Sscanf(*lastReadMessageID, "%d", &id)
		if err != nil {
			return ErrInvalidMessage
		}
		lastReadID = &id
	}

	// Update last_read_message_id in chat_members
	_, err = s.db.ExecContext(ctx, `
		UPDATE chat_members 
		SET last_read_message_id = $1 
		WHERE chat_id = $2 AND user_id = $3
	`, lastReadID, chatID, userID)
	if err != nil {
		return fmt.Errorf("failed to update last read message: %w", err)
	}

	// Clear unread count in Redis
	if lastReadMessageID != nil {
		if err := s.cacheSvc.MarkAsRead(userID, chatID, *lastReadMessageID); err != nil {
			fmt.Printf("Failed to mark messages as read in Redis: %v\n", err)
		}
	}

	return nil
}

// GetUnreadCount gets total unread messages count for a user
func (s *Service) GetUnreadCount(ctx context.Context, userID string) (map[string]int64, error) {
	// Try Redis first
	counts, err := s.cacheSvc.GetUnreadCounts(userID)
	if err == nil && len(counts) > 0 {
		return counts, nil
	}

	// Fallback to DB calculation
	return s.calculateUnreadCountsFromDB(ctx, userID)
}

// Helper functions

func (s *Service) getMessagesFromRedis(ctx context.Context, chatID string, limit int64) ([]*Message, error) {
	redisMsgs, err := s.cacheSvc.GetRecentMessages(chatID, limit)
	if err != nil {
		return nil, err
	}

	messages := make([]*Message, 0, len(redisMsgs))
	for _, redisMsg := range redisMsgs {
		msg := &Message{
			ID:        redisMsg.ID,
			ChatID:    redisMsg.ChatID,
			SenderID:  redisMsg.SenderID,
			Content:   redisMsg.Content,
			Type:      MessageType(redisMsg.Type),
			CreatedAt: redisMsg.CreatedAt,
		}

		// Set optional fields
		if redisMsg.FileURL != "" {
			msg.FileURL = &redisMsg.FileURL
		}
		if redisMsg.ReplyTo != "" {
			msg.ReplyTo = &redisMsg.ReplyTo
		}

		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *Service) getMessagesFromDB(ctx context.Context, chatID string, filter MessageFilter) ([]*Message, error) {
	query := `
		SELECT id, chat_id, message_id, sender_id, content, type, file_url, created_at
		FROM message_archive
		WHERE chat_id = $1
	`
	args := []interface{}{chatID}
	paramIndex := 2

	if filter.BeforeTime != nil {
		query += fmt.Sprintf(" AND created_at < $%d", paramIndex)
		args = append(args, *filter.BeforeTime)
		paramIndex++
	}
	if filter.AfterTime != nil {
		query += fmt.Sprintf(" AND created_at > $%d", paramIndex)
		args = append(args, *filter.AfterTime)
		paramIndex++
	}
	if filter.SenderID != nil {
		query += fmt.Sprintf(" AND sender_id = $%d", paramIndex)
		args = append(args, *filter.SenderID)
		paramIndex++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", paramIndex)
		args = append(args, filter.Limit)
		paramIndex++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", paramIndex)
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg MessageArchive
		err := rows.Scan(
			&msg.ID, &msg.ChatID, &msg.MessageID, &msg.SenderID,
			&msg.Content, &msg.Type, &msg.FileURL, &msg.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, s.convertArchiveToMessage(&msg))
	}

	return messages, nil
}

func (s *Service) verifyChatAccess(ctx context.Context, chatID, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chat_members 
			WHERE chat_id = $1 AND user_id = $2
		)
	`, chatID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to verify chat access: %w", err)
	}
	return exists, nil
}

func (s *Service) updateUnreadCounts(ctx context.Context, chatID, excludeUserID string) error {
	// Get all chat members except sender
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id FROM chat_members 
		WHERE chat_id = $1 AND user_id != $2
	`, chatID, excludeUserID)
	if err != nil {
		return fmt.Errorf("failed to get chat members: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		// Increment unread count in Redis
		if err := s.cacheSvc.IncrementUnread(userID, chatID); err != nil {
			fmt.Printf("Failed to increment unread for user %s: %v\n", userID, err)
		}
	}

	return nil
}

func (s *Service) calculateUnreadCountsFromDB(ctx context.Context, userID string) (map[string]int64, error) {
	query := `
		SELECT 
			cm.chat_id,
			COUNT(ma.id) as unread_count
		FROM chat_members cm
		LEFT JOIN message_archive ma ON 
			ma.chat_id = cm.chat_id AND 
			ma.created_at > COALESCE(
				(SELECT created_at FROM message_archive WHERE message_id = cm.last_read_message_id),
				'1970-01-01'::timestamp
			)
		WHERE cm.user_id = $1
		GROUP BY cm.chat_id
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate unread counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var chatID string
		var count int64
		if err := rows.Scan(&chatID, &count); err != nil {
			continue
		}
		counts[chatID] = count
	}

	return counts, nil
}

func (s *Service) convertArchiveToMessage(archive *MessageArchive) *Message {
	msg := &Message{
		ID:        fmt.Sprintf("%d", archive.MessageID),
		ChatID:    archive.ChatID.String(),
		SenderID:  archive.SenderID.String(),
		Content:   archive.Content,
		Type:      MessageType(archive.Type),
		CreatedAt: archive.CreatedAt,
	}

	// Set optional fields
	if archive.FileURL != nil {
		msg.FileURL = archive.FileURL
	}

	return msg
}

// Helper function to generate a message ID
func generateMessageID() int64 {
	return time.Now().UnixNano()
}
