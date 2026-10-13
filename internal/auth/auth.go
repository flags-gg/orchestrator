// Package auth carries the verified caller identity through the request context.
// The middleware in internal/auth.go is the only writer; handlers only read.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type User struct {
	ID    string
	Email string // verified primary email, empty in the development bypass
}

type ctxKey struct{}

var ErrNoUser = errors.New("no authenticated user")

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserID returns the verified user id for the request.
func UserID(r *http.Request) (string, error) {
	u, ok := r.Context().Value(ctxKey{}).(User)
	if !ok || u.ID == "" {
		return "", ErrNoUser
	}
	return u.ID, nil
}

// EmailDomain returns the lower-cased domain of the caller's verified primary email.
func EmailDomain(r *http.Request) string {
	u, _ := r.Context().Value(ctxKey{}).(User)
	_, domain, found := strings.Cut(u.Email, "@")
	if !found {
		return ""
	}
	return strings.ToLower(domain)
}
