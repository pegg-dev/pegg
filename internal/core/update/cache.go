package update

import (
	"github.com/peggco/pegg/internal/core/cache"
)

const cacheKeyDismissedVersion = "update_dismissed_version"

func GetDismissedVersion(c cache.Cache) string {
	data, err := c.Get(cacheKeyDismissedVersion)
	if err != nil {
		return ""
	}
	return string(data)
}

func SetDismissedVersion(c cache.Cache, version string) error {
	return c.Set(cacheKeyDismissedVersion, []byte(version))
}
