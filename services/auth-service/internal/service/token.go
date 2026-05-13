package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"auth-service/internal/domain"
)

type AccessClaims struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	TokenType string `json:"typ"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type TokenService struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewTokenService(secret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

func (s *TokenService) GenerateAccessToken(userID, role string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(s.accessTokenTTL)

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	claims := AccessClaims{
		UserID:    userID,
		Role:      role,
		TokenType: "access",
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := encodedHeader + "." + encodedClaims
	signature := s.sign(unsigned)

	return unsigned + "." + signature, expiresAt, nil
}

func (s *TokenService) ValidateAccessToken(token string) (AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	unsigned := parts[0] + "." + parts[1]
	expectedSignature := s.sign(unsigned)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	var claims AccessClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	if claims.TokenType != "access" || claims.UserID == "" || claims.Role == "" {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	if time.Now().UTC().Unix() >= claims.ExpiresAt {
		return AccessClaims{}, domain.ErrUnauthorized
	}

	return claims, nil
}

func (s *TokenService) GenerateRefreshToken() (rawToken string, tokenHash string, expiresAt time.Time, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}

	raw := base64.RawURLEncoding.EncodeToString(b[:])
	return raw, s.HashRefreshToken(raw), time.Now().UTC().Add(s.refreshTokenTTL), nil
}

func (s *TokenService) HashRefreshToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func (s *TokenService) RefreshTokenTTL() time.Duration {
	return s.refreshTokenTTL
}

func (s *TokenService) sign(data string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var ErrMalformedToken = errors.New("malformed token")
