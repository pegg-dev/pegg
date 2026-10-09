package shared

import (
	"github.com/peggco/pegg/internal/core/config"
)

type Deps struct {
	Config *config.Config
}

type Plugin interface {
	Name() string
	Version() string
	Description() string
	Boot(deps Deps) error
}
