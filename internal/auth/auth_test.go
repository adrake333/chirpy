package auth

import (
	"net/http"
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

func TestGetBearerToken(t *testing.T) {
    tests := []struct {
        name      string
        header    string
        wantToken string
        wantErr   bool
    }{
        {
            name:      "valid header",
            header:    "Bearer example-token",
            wantToken: "example-token",
            wantErr:   false,
        },
        {
            name:    "missing header",
            header:  "",
            wantErr: true,
        },
	{
    		name:    "wrong authorization scheme",
    		header:  "Basic example-token",
    		wantErr: true,
	},
	{
		name:		"empty token",
		header:		"Bearer ",
		wantErr: 	true,
	},
	{
		name:		"extra whitespaces",
		header:		"Bearer   example-token ",
		wantToken:	"example-token",
		wantErr:	false,
	},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            headers := make(http.Header)

            if tt.header != "" {
                headers.Set("Authorization", tt.header)
            }

            got, err := GetBearerToken(headers)

            if (err != nil) != tt.wantErr {
                t.Fatalf("GetBearerToken() error = %v, wantErr = %v", err, tt.wantErr)
            }

            if !tt.wantErr && got != tt.wantToken {
                t.Errorf("GetBearerToken() = %q, want %q", got, tt.wantToken)
            }
        })
    }
}
