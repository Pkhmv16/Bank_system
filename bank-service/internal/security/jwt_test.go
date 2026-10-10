package security

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

func TestJWTGenerateAndParse(t *testing.T) {
	user := domain.User{ID: uuid.New(), Login: "alice", Role: domain.RoleUser}
	service := NewJWT("e2d7505157241e5f50c77d61575ec0b66409a2232c0b906caabba5eedae5d00e", time.Hour)
	token, err := service.Generate(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := service.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != user.ID || claims.Role != user.Role || claims.Subject != user.Login {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
