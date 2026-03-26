package config

import (
	"testing"

	"github.com/matryer/is"
)

func TestConfig_Defaults(t *testing.T) {
	assert := is.New(t)

	cfg, err := New()
	assert.NoErr(err)
	assert.Equal(cfg.ConcurrentDownloads, 50)
}

func TestConfig_EnvOverride(t *testing.T) {
	assert := is.New(t)

	t.Setenv("PENSA_CONCURRENT_DOWNLOADS", "16")

	cfg, err := New()
	assert.NoErr(err)
	assert.Equal(cfg.ConcurrentDownloads, 16)
}
