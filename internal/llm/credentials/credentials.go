package credentials

import (
	"context"
	"errors"
	"time"
)

var ErrNotLoggedIn = errors.New("credentials: not logged in")

type Status struct {
	Provider  string
	Account   string
	Plan      string
	ExpiresAt time.Time
	LoggedIn  bool
}

type Credential interface {
	AccessToken(ctx context.Context) (string, error)
	Status() Status
}
