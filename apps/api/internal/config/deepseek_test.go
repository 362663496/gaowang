package config

import "testing"

func Test_Load_reads_optional_deepseek_configuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("DEEPSEEK_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DeepSeekAPIKey != "" || cfg.DeepSeekModel != "deepseek-v4-flash" {
		t.Fatalf("default DeepSeek config = %q/%q", cfg.DeepSeekAPIKey, cfg.DeepSeekModel)
	}

	t.Setenv("DEEPSEEK_API_KEY", "do-not-log-this-key")
	t.Setenv("DEEPSEEK_MODEL", "deepseek-v4-pro")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DeepSeekAPIKey != "do-not-log-this-key" || cfg.DeepSeekModel != "deepseek-v4-pro" {
		t.Fatalf("configured DeepSeek config = %q/%q", cfg.DeepSeekAPIKey, cfg.DeepSeekModel)
	}
}
