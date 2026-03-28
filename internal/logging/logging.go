package logging

import (
	"log/slog"
	"os"
	"strings"
)

// InitLogger will make a new text logger based on FLUFFY_LOG_LEVEL
func InitLogger() {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	if logl, ok := os.LookupEnv("FLUFFY_LOG_LEVEL"); !ok {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		switch strings.ToLower(logl) {
		case "debug", "dbg":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
		case "warn", "wrn":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})
		case "error", "err":
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})
		default:
			h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		}
	}

	// Create a new text handler that writes to the log file.
	l := slog.New(h)
	slog.SetDefault(l)
}
