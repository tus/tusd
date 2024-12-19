package handler

import (
	"crypto/rsa"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type JwtChecker struct {
	PubKey *rsa.PublicKey
}

func NewJwtChecker(pub string) (*JwtChecker, error) {
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pub))
	if err != nil {
		return nil, err
	}

	return &JwtChecker{
		PubKey: pubKey,
	}, nil
}

func (j *JwtChecker) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		rw.Write([]byte("Authorization header is required"))
		return
	}

	// we parse our jwt token and check for validity against our secret
	token, err := jwt.Parse(
		authHeader[7:],
		func(token *jwt.Token) (interface{}, error) {
			return j.PubKey, nil
		},
	)

	if err != nil {
		rw.Write([]byte("Invalid token"))
		return
	}

	if !token.Valid {
		rw.Write([]byte("Invalid token"))
		return
	}

	next(rw, r)
}
