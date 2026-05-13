package repository

import (
	"context"
	"errors"
	"fmt"

	"auth-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepository(db *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) error {
	const query = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.Exec(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.CreatedAt,
		token.RevokedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("insert refresh token: %w", err)
	}

	return nil
}

func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, created_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var token domain.RefreshToken
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshToken{}, domain.ErrNotFound
		}
		return domain.RefreshToken{}, fmt.Errorf("select refresh token: %w", err)
	}

	return token, nil
}

func (r *RefreshTokenRepository) RevokeByHash(ctx context.Context, tokenHash string) error {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`

	_, err := r.db.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}

	return nil
}

func (r *RefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID string) error {
	const query = `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL
	`

	_, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}

	return nil
}
