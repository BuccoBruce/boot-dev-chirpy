package auth

import (
	"log"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return match, err
	}

	return match, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	var (
		key []byte
		t   *jwt.Token
		s   string
	)

	key = []byte(tokenSecret)
	claims := &jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	}
	t = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString(key)
	if err != nil {
		log.Printf("Unable to create JTW token %s", err)
		return "", err
	}
	return s, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error){
	
	
	return uuid.UUID{}, nil
}