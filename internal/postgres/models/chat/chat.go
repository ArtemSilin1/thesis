package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// ChatType defines the type of chat
type ChatType string

const (
	ChatTypePrivate ChatType = "private"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"
)

// Chats represents the chat model from the image
type Chats struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      ChatType  `json:"type"`
	AvatarUrl string    `json:"avatar_url"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ChatMember represents a chat member from the image
type ChatMember struct {
	ID                uuid.UUID `json:"id"`
	ChatID            uuid.UUID `json:"chat_id"`
	UserID            uuid.UUID `json:"user_id"`
	Role              string    `json:"role"`
	JoinedAt          time.Time `json:"joined_at"`
	LastReadMessageID int       `json:"last_read_message_id"`
}

// CreateChatRequest structure for creating a chat
type CreateChatRequest struct {
	Name      string      `json:"name" validate:"required,min=1,max=100"`
	Type      ChatType    `json:"type" validate:"required,oneof=private group channel"`
	AvatarUrl string      `json:"avatar_url"`
	MemberIDs []uuid.UUID `json:"member_ids" validate:"required,min=1"`
}

// UpdateChatRequest structure for updating a chat
type UpdateChatRequest struct {
	Name      *string `json:"name,omitempty"`
	AvatarUrl *string `json:"avatar_url,omitempty"`
}

// validate checks the correctness of chat data
func (c *Chats) validate() error {
	if c.Name == "" {
		return errors.New("chat name is required")
	}
	if len(c.Name) > 100 {
		return errors.New("chat name must be less than 100 characters")
	}
	if c.Type != ChatTypePrivate && c.Type != ChatTypeGroup && c.Type != ChatTypeChannel {
		return errors.New("invalid chat type")
	}
	if c.CreatedBy == uuid.Nil {
		return errors.New("created_by is required")
	}
	return nil
}

// CreateChat creates a new chat
func (c *Chats) CreateChat(db *pgxpool.Pool, memberIDs []uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Validation
	if err := c.validate(); err != nil {
		return err
	}

	// For private chats, check if a chat already exists between these users
	if c.Type == ChatTypePrivate {
		exists, err := c.checkPrivateChatExists(db, ctx, c.CreatedBy, memberIDs[0])
		if err != nil {
			return err
		}
		if exists {
			return errors.New("private chat already exists between these users")
		}
	}

	// Start a transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Generate ID for the chat
	c.ID = uuid.New()
	c.CreatedAt = time.Now()

	// Insert the chat
	createChatQ := `
		INSERT INTO chats (id, name, type, avatar_url, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(ctx, createChatQ,
		c.ID,
		c.Name,
		c.Type,
		c.AvatarUrl,
		c.CreatedBy,
		c.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create chat: %w", err)
	}

	// Add members (including the creator)
	allMemberIDs := append([]uuid.UUID{c.CreatedBy}, memberIDs...)
	// Remove duplicates
	allMemberIDs = uniqueUUIDs(allMemberIDs)

	for _, memberID := range allMemberIDs {
		// Determine the role: creator - admin, others - member
		role := "member"
		if memberID == c.CreatedBy {
			role = "admin"
		}

		memberID := uuid.New() // Generate ID for chat_members record

		addMemberQ := `
			INSERT INTO chat_members (id, chat_id, user_id, role, joined_at, last_read_message_id)
			VALUES ($1, $2, $3, $4, $5, 0)
		`

		_, err = tx.Exec(ctx, addMemberQ, memberID, c.ID, memberID, role, c.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to add member %s: %w", memberID, err)
		}
	}

	// Commit the transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// checkPrivateChatExists checks for the existence of a private chat between two users
func (c *Chats) checkPrivateChatExists(db *pgxpool.Pool, ctx context.Context, user1ID, user2ID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM chats ch
			INNER JOIN chat_members cm1 ON cm1.chat_id = ch.id
			INNER JOIN chat_members cm2 ON cm2.chat_id = ch.id
			WHERE ch.type = 'private'
			AND cm1.user_id = $1
			AND cm2.user_id = $2
			AND cm1.chat_id = cm2.chat_id
			GROUP BY ch.id
			HAVING COUNT(DISTINCT cm1.user_id) = 2
		)
	`

	var exists bool
	err := db.QueryRow(ctx, query, user1ID, user2ID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existing private chat: %w", err)
	}

	return exists, nil
}

// GetChatByID retrieves a chat by ID
func GetChatByID(db *pgxpool.Pool, chatID uuid.UUID) (*Chats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT id, name, type, avatar_url, created_by, created_at
		FROM chats
		WHERE id = $1
	`

	var chat Chats
	err := db.QueryRow(ctx, query, chatID).Scan(
		&chat.ID,
		&chat.Name,
		&chat.Type,
		&chat.AvatarUrl,
		&chat.CreatedBy,
		&chat.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.New("chat not found")
		}
		return nil, fmt.Errorf("failed to get chat: %w", err)
	}

	return &chat, nil
}

// GetChatMembers retrieves all members of a chat
func GetChatMembers(db *pgxpool.Pool, chatID uuid.UUID) ([]ChatMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT id, chat_id, user_id, role, joined_at, last_read_message_id
		FROM chat_members
		WHERE chat_id = $1
		ORDER BY joined_at ASC
	`

	rows, err := db.Query(ctx, query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat members: %w", err)
	}
	defer rows.Close()

	var members []ChatMember
	for rows.Next() {
		var member ChatMember
		err := rows.Scan(
			&member.ID,
			&member.ChatID,
			&member.UserID,
			&member.Role,
			&member.JoinedAt,
			&member.LastReadMessageID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, member)
	}

	return members, nil
}

// GetUserChats retrieves all chats of a user
func GetUserChats(db *pgxpool.Pool, userID uuid.UUID) ([]Chats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT c.id, c.name, c.type, c.avatar_url, c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm ON cm.chat_id = c.id
		WHERE cm.user_id = $1
		ORDER BY c.created_at DESC
	`

	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user chats: %w", err)
	}
	defer rows.Close()

	var chats []Chats
	for rows.Next() {
		var chat Chats
		err := rows.Scan(
			&chat.ID,
			&chat.Name,
			&chat.Type,
			&chat.AvatarUrl,
			&chat.CreatedBy,
			&chat.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chat: %w", err)
		}
		chats = append(chats, chat)
	}

	return chats, nil
}

// UpdateChat updates chat information
func UpdateChat(db *pgxpool.Pool, chatID uuid.UUID, updates UpdateChatRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if chat exists
	_, err := GetChatByID(db, chatID)
	if err != nil {
		return err
	}

	// Build dynamic update query
	updateQ := `UPDATE chats SET `
	args := []interface{}{}
	argCount := 1

	if updates.Name != nil {
		updateQ += fmt.Sprintf("name = $%d, ", argCount)
		args = append(args, *updates.Name)
		argCount++
	}
	if updates.AvatarUrl != nil {
		updateQ += fmt.Sprintf("avatar_url = $%d, ", argCount)
		args = append(args, *updates.AvatarUrl)
		argCount++
	}

	// Remove trailing comma and space
	updateQ = updateQ[:len(updateQ)-2]
	updateQ += fmt.Sprintf(" WHERE id = $%d", argCount)
	args = append(args, chatID)

	_, err = db.Exec(ctx, updateQ, args...)
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}

	return nil
}

// DeleteChat deletes a chat
func DeleteChat(db *pgxpool.Pool, chatID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start a transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete chat members
	deleteMembersQ := `DELETE FROM chat_members WHERE chat_id = $1`
	_, err = tx.Exec(ctx, deleteMembersQ, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete chat members: %w", err)
	}

	// Delete the chat itself
	deleteChatQ := `DELETE FROM chats WHERE id = $1`
	_, err = tx.Exec(ctx, deleteChatQ, chatID)
	if err != nil {
		return fmt.Errorf("failed to delete chat: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// AddMember adds a member to a chat
func AddMember(db *pgxpool.Pool, chatID, userID uuid.UUID, role string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if the user is already a member
	var exists bool
	checkQ := `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`
	err := db.QueryRow(ctx, checkQ, chatID, userID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check existing member: %w", err)
	}
	if exists {
		return errors.New("user is already a member of this chat")
	}

	memberID := uuid.New()

	addQ := `
		INSERT INTO chat_members (id, chat_id, user_id, role, joined_at, last_read_message_id)
		VALUES ($1, $2, $3, $4, $5, 0)
	`

	_, err = db.Exec(ctx, addQ, memberID, chatID, userID, role, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
}

// RemoveMember removes a member from a chat
func RemoveMember(db *pgxpool.Pool, chatID, userID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	removeQ := `DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`

	_, err := db.Exec(ctx, removeQ, chatID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	return nil
}

// UpdateLastReadMessage updates the last read message ID for a user in a chat
func UpdateLastReadMessage(db *pgxpool.Pool, chatID, userID uuid.UUID, messageID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateQ := `
		UPDATE chat_members
		SET last_read_message_id = $1
		WHERE chat_id = $2 AND user_id = $3
	`

	_, err := db.Exec(ctx, updateQ, messageID, chatID, userID)
	if err != nil {
		return fmt.Errorf("failed to update last_read_message_id: %w", err)
	}

	return nil
}

// uniqueUUIDs removes duplicates from a slice of UUIDs
func uniqueUUIDs(uuids []uuid.UUID) []uuid.UUID {
	keys := make(map[uuid.UUID]bool)
	list := []uuid.UUID{}
	for _, entry := range uuids {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// SearchChats searches for chats by name
func SearchChats(db *pgxpool.Pool, userID uuid.UUID, query string) ([]Chats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	searchQ := `
		SELECT DISTINCT c.id, c.name, c.type, c.avatar_url, c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm ON cm.chat_id = c.id
		WHERE cm.user_id = $1 AND c.name ILIKE '%' || $2 || '%'
		ORDER BY c.name
		LIMIT 50
	`

	rows, err := db.Query(ctx, searchQ, userID, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search chats: %w", err)
	}
	defer rows.Close()

	var chats []Chats
	for rows.Next() {
		var chat Chats
		err := rows.Scan(
			&chat.ID,
			&chat.Name,
			&chat.Type,
			&chat.AvatarUrl,
			&chat.CreatedBy,
			&chat.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chat: %w", err)
		}
		chats = append(chats, chat)
	}

	return chats, nil
}
