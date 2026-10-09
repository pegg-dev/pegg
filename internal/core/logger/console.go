package logger

import (
	"fmt"

	"github.com/peggco/pegg/internal/core/config"
)

const DriverConsole = "console"

func init() {
	RegisterDriver(DriverConsole, func(cfg config.LoggerConfig) (Handler, error) {
		return NewConsoleHandler(cfg), nil
	})
}

type ConsoleHandler struct{}

func NewConsoleHandler(cfg config.LoggerConfig) *ConsoleHandler {
	return &ConsoleHandler{}
}

func (c *ConsoleHandler) Write(rec Record) error {
	fmt.Printf("[%s] [%-5s] %s\n",
		rec.Timestamp.Format("2006-01-02 15:04:05"),
		rec.Level.String(),
		rec.Message,
	)
	return nil
}

func (c *ConsoleHandler) Close() error {
	return nil
}
