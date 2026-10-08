package config

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func ParseToken(textToken string) (token *jwt.Token, err error) {
	parser := new(jwt.Parser)
	token, _, err = parser.ParseUnverified(textToken, jwt.MapClaims{})
	if err != nil {
		return
	}
	return token, nil
}

func TokenExpired(textToken string) (expired bool, err error) {
	parsed, err := ParseToken(textToken)
	if err != nil {
		return false, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		err = fmt.Errorf("expected map claims but got %T", claims)
		return
	}
	claim, ok := claims["exp"]
	if !ok {
		return false, nil
	}
	exp, ok := claim.(float64)
	if !ok {
		err = fmt.Errorf("expected floating point 'exp' but got %T", claim)
		return
	}
	if exp == 0 {
		return false, nil
	}
	left := time.Until(time.Unix(int64(exp), 0))
	expired = left < 5*time.Second
	return
}

// tokenExpiry returns the 'exp' claim of a JWT without verifying it. The claims
// are display and scheduling hints only, never an authorization decision.
func tokenExpiry(textToken string) (time.Time, bool) {
	parsed, err := ParseToken(textToken)
	if err != nil {
		return time.Time{}, false
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, false
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}
