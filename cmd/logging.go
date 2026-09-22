package cmd

import (
	"io"
	"log/slog"
	"strings"
)

const logLevelOff = slog.Level(100)

func configureLogging(logxi string, output io.Writer) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: parseLogLevel(logxi),
	})))
}

func parseLogLevel(value string) slog.Level {
	level := slog.LevelError
	for expression := range strings.SplitSeq(value, ",") {
		expression = strings.ToUpper(strings.TrimSpace(expression))
		if expression == "" {
			continue
		}

		name, configuredLevel, hasAssignment := strings.Cut(expression, "=")
		if hasAssignment {
			if strings.TrimSpace(name) != "*" {
				continue
			}
			expression = strings.TrimSpace(configuredLevel)
		} else if expression != "*" && expression != "-*" {
			continue
		}

		parsed, ok := parseGlobalLogLevel(expression)
		if !ok {
			level = slog.LevelError
			continue
		}
		level = parsed
	}
	return level
}

func parseGlobalLogLevel(value string) (slog.Level, bool) {
	switch value {
	case "*", "ALL", "TRC", "DBG", "DEBUG":
		return slog.LevelDebug, true
	case "INF", "INFO":
		return slog.LevelInfo, true
	case "WRN", "WARN":
		return slog.LevelWarn, true
	case "ERR", "ERROR", "FTL", "FATAL":
		return slog.LevelError, true
	case "OFF", "-*":
		return logLevelOff, true
	default:
		return slog.LevelError, false
	}
}
