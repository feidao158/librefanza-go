package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("account already exists")
)

type User struct {
	ID           uint64    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"-"`
}

type session struct {
	ID               uint64
	UserID           uint64
	AccessHash       string
	RefreshHash      string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (session) TableName() string { return "api_sessions" }

type resetToken struct {
	ID        uint64
	UserID    uint64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (resetToken) TableName() string { return "password_reset_tokens" }

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	User         User   `json:"user"`
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 320 || len(password) < 8 || len(password) > 72 {
		return User{}, ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, ErrInvalidInput
	}
	user := User{Email: email, PasswordHash: string(hash)}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		var count int64
		if s.db.WithContext(ctx).Model(&User{}).Where("email = ?", email).Count(&count).Error == nil && count > 0 {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (Tokens, error) {
	var user User
	err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Tokens{}, ErrUnauthorized
	}
	access, refresh, err := newTokens()
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	record := session{UserID: user.ID, AccessHash: tokenHash(access), RefreshHash: tokenHash(refresh), AccessExpiresAt: now.Add(15 * time.Minute), RefreshExpiresAt: now.Add(30 * 24 * time.Hour)}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900, User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthorized
	}
	var record session
	if err := s.db.WithContext(ctx).Where("access_hash = ? AND access_expires_at > ?", tokenHash(token), time.Now().UTC()).First(&record).Error; err != nil {
		return User{}, ErrUnauthorized
	}
	var user User
	if err := s.db.WithContext(ctx).First(&user, record.UserID).Error; err != nil {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

func (s *Service) Refresh(ctx context.Context, token string) (Tokens, error) {
	if token == "" {
		return Tokens{}, ErrUnauthorized
	}
	access, refresh, err := newTokens()
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	var user User
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&session{}).Where("refresh_hash = ? AND refresh_expires_at > ?", tokenHash(token), now).Updates(map[string]any{
			"access_hash": tokenHash(access), "refresh_hash": tokenHash(refresh), "access_expires_at": now.Add(15 * time.Minute), "refresh_expires_at": now.Add(30 * 24 * time.Hour),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrUnauthorized
		}
		var record session
		if err := tx.Where("refresh_hash = ?", tokenHash(refresh)).First(&record).Error; err != nil {
			return err
		}
		return tx.First(&user, record.UserID).Error
	})
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900, User: user}, nil
}

func (s *Service) Logout(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return ErrUnauthorized
	}
	return s.db.WithContext(ctx).Where("access_hash = ?", tokenHash(accessToken)).Delete(&session{}).Error
}

func (s *Service) CreatePasswordReset(ctx context.Context, email string) (string, error) {
	var user User
	err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	token, _, err := newTokens()
	if err != nil {
		return "", err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&resetToken{}).Error; err != nil {
			return err
		}
		return tx.Create(&resetToken{UserID: user.ID, TokenHash: tokenHash(token), ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC()}).Error
	})
	return token, err
}

func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if token == "" || len(password) < 8 || len(password) > 72 {
		return ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ErrInvalidInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry resetToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ? AND expires_at > ?", tokenHash(token), time.Now().UTC()).First(&entry).Error; err != nil {
			return ErrUnauthorized
		}
		if err := tx.Model(&User{}).Where("id = ?", entry.UserID).Update("password_hash", string(hash)).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", entry.UserID).Delete(&session{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", entry.UserID).Delete(&resetToken{}).Error
	})
}

func newTokens() (string, string, error) {
	values := [2]string{}
	for i := range values {
		var data [32]byte
		if _, err := rand.Read(data[:]); err != nil {
			return "", "", err
		}
		values[i] = base64.RawURLEncoding.EncodeToString(data[:])
	}
	return values[0], values[1], nil
}

func tokenHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
