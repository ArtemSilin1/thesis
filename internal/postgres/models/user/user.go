package user

import (
	"context"
	"errors"
	"fmt"
	"thesis/internal/postgres/data"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Users struct {
	Id            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
	Bio           string    `json:"bio"`
	AvatarUrl     string    `json:"avatar_url"`
	LastSeen      time.Time `json:"last_seen"`
	CreatedAt     time.Time `json:"created"`
}

type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var jwtSecret = []byte("your-secret-key-change-in-production")

func (u *Users) toString(data ...any) string {
	return fmt.Sprint(data)
}

// Returning "false" if not exist
// or "true" if exist
func (u *Users) checkUserExist(db *pgxpool.Pool, ctx context.Context) bool {
	var user int
	if err := db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
		u.Email,
		u.Username,
	).Scan(&user); err != nil {
		return false
	}

	return user > 0
}

func (u *Users) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (u *Users) checkPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}

func (u *Users) CreateAcc(db *pgxpool.Pool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := u.validateUserData(); err != nil {
		return "", err
	}

	isUserExist := u.checkUserExist(db, ctx)
	if isUserExist {
		return "", data.ErrExists
	}

	hashedPassword, err := u.hashPassword(u.Password)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	createAccQ := `
		INSERT INTO users(email, username, password, bio) 
		VALUES($1, $2, $3, $4, $5)
		RETURNING id;
	`

	var createdUserId uuid.UUID
	if err := db.QueryRow(
		ctx,
		createAccQ,
		u.Email,
		u.Username,
		hashedPassword,
		u.Bio,
	).Scan(&createdUserId); err != nil {
		return "", fmt.Errorf("failed to create user: %w", err)
	}

	u.Id = createdUserId
	token, err := u.generateJWTToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

func (u *Users) Login(db *pgxpool.Pool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loginQ := `
		SELECT id, email, username, password, bio, avatar_url, email_verified, created_at, last_seen
		FROM users 
		WHERE email = $1 OR username = $1;
	`

	var dbUser Users
	err := db.QueryRow(ctx, loginQ, u.Email).Scan(
		&dbUser.Id,
		&dbUser.Email,
		&dbUser.Username,
		&dbUser.Password,
		&dbUser.Bio,
		&dbUser.AvatarUrl,
		&dbUser.EmailVerified,
		&dbUser.CreatedAt,
		&dbUser.LastSeen,
	)

	if err != nil {
		return "", errors.New("invalid credentials")
	}

	if err = dbUser.checkPassword(u.Password); err != nil {
		return "", errors.New("invalid credentials")
	}

	updateLastSeenQ := `UPDATE users SET last_seen = $1 WHERE id = $2;`
	_, err = db.Exec(ctx, updateLastSeenQ, time.Now(), dbUser.Id)
	if err != nil {
		fmt.Printf("Failed to update last_seen: %v\n", err)
	}

	token, err := dbUser.generateJWTToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

func (u *Users) CheckEmailAuth() error {
	// TODO: Реализовать проверку email через отправку кода подтверждения
	// 1. Генерировать случайный код
	// 2. Сохранить код в БД/кэш с временем жизни
	// 3. Отправить код на email пользователя
	// 4. При верификации проверить код и установить EmailVerified = true
	return nil
}

// validation
func (u *Users) validateUserData() error {
	if u.Email == "" {
		return errors.New("email is required")
	}
	if u.Username == "" {
		return errors.New("username is required")
	}
	if len(u.Username) < 3 || len(u.Username) > 30 {
		return errors.New("username must be between 3 and 30 characters")
	}
	if u.Password == "" {
		return errors.New("password is required")
	}
	if len(u.Password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	return nil
}

// Token func
func (u *Users) generateJWTToken() (string, error) {
	expirationTime := time.Now().Add(72 * time.Hour)

	claims := &Claims{
		UserID:   u.Id.String(),
		Email:    u.Email,
		Username: u.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "thesis-app",
			Subject:   u.Id.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func DecodeJWTToken(tokenString string) (*Users, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token: %w", err)
	}

	user := &Users{
		Id:       userID,
		Email:    claims.Email,
		Username: claims.Username,
	}

	return user, nil
}

// --
func GetUserByID(db *pgxpool.Pool, userID uuid.UUID) (*Users, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT id, email, username, bio, avatar_url, email_verified, created_at, last_seen
		FROM users 
		WHERE id = $1;
	`

	var user Users
	err := db.QueryRow(ctx, query, userID).Scan(
		&user.Id,
		&user.Email,
		&user.Username,
		&user.Bio,
		&user.AvatarUrl,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.LastSeen,
	)

	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// UpdateProfile - user profile data update
func (u *Users) UpdateProfile(db *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updateQ := `
		UPDATE users 
		SET username = $1, bio = $2, avatar_url = $3
		WHERE id = $4;
	`

	_, err := db.Exec(ctx, updateQ, u.Username, u.Bio, u.AvatarUrl, u.Id)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	return nil
}
