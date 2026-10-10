package security

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

type Claims struct {
	UserID uuid.UUID   `json:"userId"`
	Role   domain.Role `json:"role"`
	jwt.RegisteredClaims
}

type JWT struct {
	key        []byte
	expiration time.Duration
}

func NewJWT(secret string, expiration time.Duration) *JWT {
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) < 32 {
		key = []byte(secret)
	}
	return &JWT{key: key, expiration: expiration}
}

func (j *JWT) Generate(user domain.User) (string, error) {
	now := time.Now()
	claims := Claims{UserID: user.ID, Role: user.Role, RegisteredClaims: jwt.RegisteredClaims{
		Subject: user.Login, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(j.expiration)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.key)
}

func (j *JWT) Parse(raw string) (Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return j.key, nil
	})
	if err != nil || !token.Valid {
		return Claims{}, fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return Claims{}, fmt.Errorf("invalid claims")
	}
	return *claims, nil
}
