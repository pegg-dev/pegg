package notification

import (
	"github.com/gen2brain/beeep"

	"github.com/peggco/pegg/internal/core/config"
)

const DriverOS = "os"

func init() {
	RegisterDriver(DriverOS, func(cfg config.NotificationConfig) (Driver, error) {
		return &OSDriver{}, nil
	})
}

type OSDriver struct{}

func (d *OSDriver) Notify(n Notification) error {
	beeep.AppName = config.AppName
	return beeep.Notify(n.Title, n.Message, "")
}

func (d *OSDriver) Close() error {
	return nil
}
