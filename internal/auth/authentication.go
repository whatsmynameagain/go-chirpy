package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("error hashing password: %w", err)
	}
	return string(hashed), nil
}

func CheckPasswordHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

type CustomClaims struct {
	jwt.RegisteredClaims
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := CustomClaims{
		jwt.RegisteredClaims{
			Issuer:    "chirpy",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	// Validate JWT format
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return uuid.Nil, fmt.Errorf("invalid JWT: must have 3 parts")
	}

	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		// Debug header on error
		header, decodeErr := base64.RawURLEncoding.DecodeString(parts[0])
		if decodeErr != nil {
			return uuid.Nil, fmt.Errorf("failed to decode header: %w", err)
		}
		return uuid.Nil, fmt.Errorf("failed to parse token: %w, header: %s", err, string(header))
	}

	// Validate claims and token
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		userID, err := claims.GetSubject()
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid subject: %w", err)
		}

		issuer, err := claims.GetIssuer()
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid issuer: %w", err)
		}
		if issuer != "chirpy" {
			return uuid.Nil, errors.New("invalid issuer")
		}

		userUUID, err := uuid.Parse(userID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("invalid user id: %w", err)
		}
		return userUUID, nil
	}

	return uuid.Nil, errors.New("invalid claims or token")
}

// untested
func GetBearerToken(headers http.Header) (string, error) {
	auth_header := headers.Get("Authorization")
	if auth_header == "" {
		return "", fmt.Errorf("no authorizaton header found")
	}

	token_string := strings.Split(auth_header, " ")[1]

	return token_string, nil
}
