package subscription

import (
	"sort"

	"github.com/peggco/pegg/internal/llm/credentials"
)

type Info struct {
	Provider string
	Hint     string
	Status   func() credentials.Status
}

var registry = map[string]Info{}

func Register(info Info) {
	if info.Provider == "" {
		return
	}
	registry[info.Provider] = info
}

func Get(name string) (Info, bool) {
	info, ok := registry[name]
	return info, ok
}

func IsSubscription(name string) bool {
	_, ok := registry[name]
	return ok
}

func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
