package wsforwarder

import (
	"os"

	"golang.org/x/exp/slog"
)

var LogLevel = new(slog.LevelVar)
var logger *slog.Logger

func init() {
	LogLevel.Set(slog.LevelWarn)

	th := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: LogLevel})
	logger = slog.New(th).WithGroup("wsforwarder")
}
