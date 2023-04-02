package wsforwarder

import (
	"os"

	"golang.org/x/exp/slog"
)

var LogLevel = new(slog.LevelVar)
var logger *slog.Logger

func init() {
	LogLevel.Set(slog.LevelWarn)

	logger = slog.New(slog.HandlerOptions{Level: LogLevel}.
		NewTextHandler(os.Stderr)).
		WithGroup("wsforwarder")
}
