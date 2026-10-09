package middlewares

import (
	"github.com/peggco/pegg/internal/agent/middleware"
)

var reg = middleware.NewMiddlewareRegistry()

func Register(name string, m middleware.Middleware) error {
	return reg.Register(name, m)
}

func Unregister(name string) bool {
	return reg.Unregister(name)
}

func Get(name string) (middleware.Middleware, bool) {
	return reg.Get(name)
}

func Names() []string {
	return reg.Names()
}

func List() []middleware.NamedMiddleware {
	return reg.List()
}
