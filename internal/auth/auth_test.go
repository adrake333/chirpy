package auth




import (
	"testing"
	"time"

	"github.com/google/uuid"
)




func TestTokenValid(t *testing.T) {
	originalID := uuid.New()
	secret := "test-secret"
	expiresIn := 1 * time.Hour
	tokenString, err := MakeJWT(originalID, secret, expiresIn)
	if err != nil {
		t.Fatalf("expected no error creating JWT: %v", err)
	}
	gotID, err := ValidateJWT(tokenString, secret)
	if err != nil {
		t.Fatalf("expected no error validating JWT: %v", err)
	}
	if gotID != originalID {
		t.Errorf("expected ID %v, got %v", originalID, gotID)
	}
}

func TestExpired(t *testing.T) {
	originalID := uuid.New()
	secret := "test-secret"
	expiresIn := -1 * time.Hour
	tokenString, err := MakeJWT(originalID, secret, expiresIn)
	if err != nil {
		t.Fatalf("expected no error creating JWT: %v", err)
	}
	_, err = ValidateJWT(tokenString, secret)
	if err == nil {
		t.Fatal("expected error validating expired token")
	}
}

func TestInvalidSigned(t *testing.T) {
	originalID := uuid.New()
	createSecret := "test-secret"
	validateSecret := "wrong-secret"
	expiresIn := 1 * time.Hour
	tokenString, err := MakeJWT(originalID, createSecret, expiresIn)
	if err != nil {
		t.Fatalf("expected no error creating JWT: %v", err)
	}
	_, err = ValidateJWT(tokenString, validateSecret)
	if err == nil {
		t.Fatal("expected error validating token secret")
	}
}
