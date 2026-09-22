package cmd

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "empty", value: "", want: slog.LevelError},
		{name: "asterisk", value: "*", want: slog.LevelDebug},
		{name: "all", value: "*=ALL", want: slog.LevelDebug},
		{name: "trace", value: "*=TRC", want: slog.LevelDebug},
		{name: "debug short", value: "*=DBG", want: slog.LevelDebug},
		{name: "debug", value: "*=DEBUG", want: slog.LevelDebug},
		{name: "info short", value: "*=INF", want: slog.LevelInfo},
		{name: "info", value: "*=INFO", want: slog.LevelInfo},
		{name: "warn short", value: "*=WRN", want: slog.LevelWarn},
		{name: "warn", value: "*=WARN", want: slog.LevelWarn},
		{name: "error short", value: "*=ERR", want: slog.LevelError},
		{name: "error", value: "*=ERROR", want: slog.LevelError},
		{name: "fatal short", value: "*=FTL", want: slog.LevelError},
		{name: "fatal", value: "*=FATAL", want: slog.LevelError},
		{name: "off", value: "*=OFF", want: logLevelOff},
		{name: "case insensitive", value: "*=info", want: slog.LevelInfo},
		{name: "named ignored", value: "named=INF", want: slog.LevelError},
		{name: "last global wins", value: "*=DEBUG,named=ERROR,*=WARN,other=OFF", want: slog.LevelWarn},
		{name: "last malformed global falls back", value: "*=DEBUG,*=NOPE", want: slog.LevelError},
		{name: "malformed", value: "NOPE", want: slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLogLevel(tt.value); got != tt.want {
				t.Fatalf("parseLogLevel(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestConfigureLoggingOff(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	var output bytes.Buffer
	configureLogging("*=OFF", &output)
	slog.Error("hidden")
	if output.Len() != 0 {
		t.Fatalf("OFF log output = %q, want empty", output.String())
	}
}

func TestConfigureLoggingDefaultAndVerbose(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	var output bytes.Buffer
	configureLogging("", &output)
	slog.Info("hidden")
	slog.Error("visible", "program", "cron")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("default log output is not JSON: %v", err)
	}
	if record["msg"] != "visible" || record["program"] != "cron" || record["level"] != "ERROR" {
		t.Fatalf("default log record = %#v", record)
	}

	output.Reset()
	configureLogging("*", &output)
	slog.Info("visible info", "program", "cron")
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("verbose log output is not JSON: %v", err)
	}
	if record["msg"] != "visible info" || record["level"] != "INFO" {
		t.Fatalf("verbose log record = %#v", record)
	}
}
