package update

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"

	"github.com/peggco/pegg/internal/core/config"
)

const repoSlug = "peggco/pegg"

type Release struct {
	Version string
}

func DetectLatest(ctx context.Context) (*Release, bool, error) {
	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug(repoSlug))
	if err != nil {
		return nil, false, fmt.Errorf("detect latest: %w", err)
	}
	if !found {
		return nil, false, nil
	}
	return &Release{
		Version: latest.Version(),
	}, true, nil
}

func IsNewerThan(current, latest string) bool {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return false
	}
	lat, err := semver.NewVersion(latest)
	if err != nil {
		return false
	}
	return lat.GreaterThan(cur)
}

func UpdateToLatest(ctx context.Context) error {
	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug(repoSlug))
	if err != nil {
		return fmt.Errorf("detect latest: %w", err)
	}
	if !found {
		return fmt.Errorf("no release found")
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if err := selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		return fmt.Errorf("update binary: %w", err)
	}

	return nil
}

func CurrentVersion() string {
	return config.AppVersion
}
