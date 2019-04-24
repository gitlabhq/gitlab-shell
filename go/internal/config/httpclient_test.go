package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTimeout(t *testing.T) {
	expectedSeconds := uint64(300)

	config := &Config{
		GitlabUrl:    "http://localhost:3000",
		HttpSettings: HttpSettingsConfig{ReadTimeoutSeconds: expectedSeconds},
	}
	client := config.GetHttpClient()

	require.NotNil(t, client)
	assert.Equal(t, time.Duration(expectedSeconds)*time.Second, client.HttpClient.Timeout)
}
