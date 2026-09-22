package cmd

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{value: "", want: slog.LevelError},
		{value: "*", want: slog.LevelDebug},
		{value: "*=INF", want: slog.LevelInfo},
		{value: "*=WRN", want: slog.LevelWarn},
		{value: "*=ERR", want: slog.LevelError},
		{value: "named=INF", want: slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := parseLogLevel(tt.value); got != tt.want {
				t.Fatalf("parseLogLevel(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
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
