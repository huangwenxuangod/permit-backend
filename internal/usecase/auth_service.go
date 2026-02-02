package usecase

import (
	"time"
	"permit-backend/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

type UserRepo interface {
	PutUser(*domain.User) error
	GetUserByOpenID(openid string) (*domain.User, bool)
}

type WechatClient interface {
	Jscode2Session(code string) (string, string, error)
}

type AuthService struct {
	Repo      UserRepo
	Wechat    WechatClient
	JWTSecret string
}

func (s *AuthService) Login(code string) (string, *domain.User, error) {
	openid, _, err := s.Wechat.Jscode2Session(code)
	if err != nil {
		return "", nil, err
	}
	u, ok := s.Repo.GetUserByOpenID(openid)
	if !ok {
		now := time.Now().UTC()
		u = &domain.User{
			UserID:    randomID(),
			OpenID:    openid,
			Nickname:  "",
			Avatar:    "",
			CreatedAt: now,
			UpdatedAt: now,
		}
		_ = s.Repo.PutUser(u)
	}
	claims := jwt.MapClaims{
		"user_id": u.UserID,
		"openid":  u.OpenID,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.JWTSecret))
	if err != nil {
		return "", nil, err
	}
	return signed, u, nil
}

func (s *AuthService) Verify(token string) (string, string, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return []byte(s.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		return "", "", err
	}
	m, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", ErrNotFound("claims")
	}
	uid, _ := m["user_id"].(string)
	oid, _ := m["openid"].(string)
	return uid, oid, nil
}
