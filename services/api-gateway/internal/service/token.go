package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"api-gateway/internal/domain"
)

type TokenValidator struct {
	secret []byte
}

func NewTokenValidator(secret string) *TokenValidator {
	return &TokenValidator{secret: []byte(secret)}
}

func (v *TokenValidator) ValidateAccessToken(token string) (domain.AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.AccessClaims{}, domain.ErrUnauthorized
	}

	unsigned := parts[0] + "." + parts[1]
	expectedSignature := v.sign(unsigned)
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

func (v *TokenValidator) sign(data string) string {
	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
