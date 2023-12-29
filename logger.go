package cutejaguar

import (
	"log/slog"
	"os"

	"github.com/taylormonacelli/littlecow"
)

func getLogger(logLevel slog.Level, logFormat string) (*slog.Logger, error) {
	opts := littlecow.NewHandlerOptions(logLevel, littlecow.RemoveTimestampAndTruncateSource)

	var handler slog.Handler
	handler = slog.NewTextHandler(os.Stderr, opts)
	if logFormat == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler), nil
}

func setupLogger() error {
	logger, err := getLogger(opts.logLevel, opts.LogFormat)
	if err != nil {
		slog.Error("getLogger", "error", err)
		return err
	}
	slog.SetDefault(logger)
	return nil
}

func setLogLevel() error {
	switch {
	case len(opts.Verbose) >= 2:
		opts.logLevel = slog.LevelDebug
	case len(opts.Verbose) == 1:
		opts.logLevel = slog.LevelInfo
	default:
		opts.logLevel = slog.LevelWarn
	}
	return nil
}
