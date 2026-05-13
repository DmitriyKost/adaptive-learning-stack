package domain

type AccessClaims struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	TokenType string `json:"typ"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

const RoleAdmin = "admin"
