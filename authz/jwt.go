package authz

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ValidateToken checks an HS256 token against the shared secret and returns
// its claims. Tokens with any other algorithm, or without an exp claim, are rejected.
func ValidateToken(token string, signedJWTKey string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (interface{}, error) {
			return []byte(signedJWTKey), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	return claims, nil
}
