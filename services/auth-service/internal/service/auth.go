package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"auth-service/internal/domain"
)

type UserStore interface {
	CreateUser(ctx context.Context, user domain.User) error
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id string) (domain.User, error)
	UpdateUser(ctx context.Context, user domain.User) error
}

type RefreshTokenStore interface {
	Create(ctx context.Context, token domain.RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error)
	RevokeByHash(ctx context.Context, tokenHash string) error
	RevokeAllByUserID(ctx context.Context, userID string) error
}

type AuthUsecase struct {
	users         UserStore
	refreshTokens RefreshTokenStore
	passwords     *PasswordService
	tokens        *TokenService
}

func NewAuthUsecase(
	users UserStore,
	refreshTokens RefreshTokenStore,
	passwords *PasswordService,
	tokens *TokenService,
) *AuthUsecase {
	return &AuthUsecase{
		users:         users,
		refreshTokens: refreshTokens,
		passwords:     passwords,
		tokens:        tokens,
	}
}

func (u *AuthUsecase) Register(ctx context.Context, email, password string) (domain.AuthResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return domain.AuthResult{}, err
	}
	if err := validatePassword(password); err != nil {
		return domain.AuthResult{}, err
	}

	passwordHash, err := u.passwords.HashPassword(password)
	if err != nil {
		return domain.AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	userID, err := domain.NewUUID()
	if err != nil {
		return domain.AuthResult{}, fmt.Errorf("generate user id: %w", err)
	}

	user := domain.User{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         domain.RoleStudent,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := u.users.CreateUser(ctx, user); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.AuthResult{}, domain.ErrConflict
		}
		return domain.AuthResult{}, err
	}

	return u.issueTokens(ctx, user)
}

func (u *AuthUsecase) Login(ctx context.Context, email, password string) (domain.AuthResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return domain.AuthResult{}, domain.ErrUnauthorized
	}

	user, err := u.users.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AuthResult{}, domain.ErrUnauthorized
		}
		return domain.AuthResult{}, err
	}

	if !u.passwords.ComparePassword(user.PasswordHash, password) {
		return domain.AuthResult{}, domain.ErrUnauthorized
	}

	return u.issueTokens(ctx, user)
}

func (u *AuthUsecase) Refresh(ctx context.Context, rawRefreshToken string) (domain.AuthResult, error) {
	rawRefreshToken = strings.TrimSpace(rawRefreshToken)
	if rawRefreshToken == "" {
		return domain.AuthResult{}, domain.ErrUnauthorized
	}

	tokenHash := u.tokens.HashRefreshToken(rawRefreshToken)
	storedToken, err := u.refreshTokens.GetByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AuthResult{}, domain.ErrUnauthorized
		}
		return domain.AuthResult{}, err
	}

	if storedToken.RevokedAt != nil || time.Now().UTC().After(storedToken.ExpiresAt) {
		return domain.AuthResult{}, domain.ErrUnauthorized
	}

	if err := u.refreshTokens.RevokeByHash(ctx, tokenHash); err != nil {
		return domain.AuthResult{}, err
	}

	user, err := u.users.GetUserByID(ctx, storedToken.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.AuthResult{}, domain.ErrUnauthorized
		}
		return domain.AuthResult{}, err
	}

	return u.issueTokens(ctx, user)
}

func (u *AuthUsecase) Logout(ctx context.Context, rawRefreshToken string) error {
	rawRefreshToken = strings.TrimSpace(rawRefreshToken)
	if rawRefreshToken == "" {
		return nil
	}

	tokenHash := u.tokens.HashRefreshToken(rawRefreshToken)
	return u.refreshTokens.RevokeByHash(ctx, tokenHash)
}

func (u *AuthUsecase) GetUserByID(ctx context.Context, userID string) (domain.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.User{}, domain.ErrInvalidInput
	}
	return u.users.GetUserByID(ctx, userID)
}

func (u *AuthUsecase) issueTokens(ctx context.Context, user domain.User) (domain.AuthResult, error) {
	accessToken, accessExpiresAt, err := u.tokens.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return domain.AuthResult{}, fmt.Errorf("generate access token: %w", err)
	}

	rawRefreshToken, refreshHash, refreshExpiresAt, err := u.tokens.GenerateRefreshToken()
	if err != nil {
		return domain.AuthResult{}, err
	}

	refreshID, err := domain.NewUUID()
	if err != nil {
		return domain.AuthResult{}, fmt.Errorf("generate refresh token id: %w", err)
	}

	refreshToken := domain.RefreshToken{
		ID:        refreshID,
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: refreshExpiresAt,
		CreatedAt: time.Now().UTC(),
	}

	if err := u.refreshTokens.Create(ctx, refreshToken); err != nil {
		return domain.AuthResult{}, err
	}

	return domain.AuthResult{
		User: user,
		Tokens: domain.AuthTokens{
			AccessToken:           accessToken,
			RefreshToken:          rawRefreshToken,
			AccessTokenExpiresAt:  accessExpiresAt,
			RefreshTokenExpiresAt: refreshExpiresAt,
		},
	}, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(email) > 320 {
		return "", domain.ErrInvalidInput
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", domain.ErrInvalidInput
	}

	return email, nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return domain.ErrInvalidInput
	}
	return nil
}
