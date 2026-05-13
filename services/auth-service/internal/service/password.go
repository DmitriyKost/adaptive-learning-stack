package service

import "golang.org/x/crypto/bcrypt"

type PasswordService struct {
	cost int
}

func NewPasswordService(cost int) *PasswordService {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &PasswordService{cost: cost}
}

func (s *PasswordService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *PasswordService) ComparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
