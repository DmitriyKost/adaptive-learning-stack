package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"playground-service/internal/domain"
)

type TokenService struct {
	secret []byte
}

func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret)}
}

func (s *TokenService) ValidateAccessToken(token string) (domain.AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	unsigned := parts[0] + "." + parts[1]
	expectedSignature := s.sign(unsigned)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	var claims domain.AccessClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	if claims.TokenType != "access" || claims.UserID == "" || claims.Role == "" {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	if time.Now().UTC().Unix() >= claims.ExpiresAt {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	return claims, nil
}

func (s *TokenService) sign(data string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
