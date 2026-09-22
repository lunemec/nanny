package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInitConfigSourcesAndPrecedence(t *testing.T) {
	resetConfigState(t)

	configPath := filepath.Join(t.TempDir(), "nanny.toml")
	contents := []byte(`
name = "from-file"
addr = "localhost:9999"
storage_dsn = "file:file.sqlite"

[stderr]
enabled = false
`)
	if err := os.WriteFile(configPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	cfgFile = configPath
	t.Setenv("NANNY_NAME", "from-env")
	t.Setenv("NANNY_STORAGE_DSN", "file:env.sqlite")
	initConfig()

	if config.Name != "from-env" {
		t.Fatalf("Name = %q, want environment value", config.Name)
	}
	if config.Addr != "localhost:9999" {
		t.Fatalf("Addr = %q, want config file value", config.Addr)
	}
	if config.StorageDSN != "file:env.sqlite" {
		t.Fatalf("StorageDSN = %q, want environment value", config.StorageDSN)
	}
	if config.Stderr.Enabled {
		t.Fatal("Stderr.Enabled = true, want config file value false")
	}
}

func TestInitConfigMissingFileUsesStderr(t *testing.T) {
	resetConfigState(t)

	cfgFile = filepath.Join(t.TempDir(), "missing.toml")
	initConfig()

	if !config.Stderr.Enabled {
		t.Fatal("Stderr.Enabled = false, want fallback true")
	}
}

func resetConfigState(t *testing.T) {
	t.Helper()
	previousConfig := config
	previousCfgFile := cfgFile

	viper.Reset()
	config = Config{}
	cfgFile = ""
	t.Cleanup(func() {
		viper.Reset()
		config = previousConfig
		cfgFile = previousCfgFile
	})
}
