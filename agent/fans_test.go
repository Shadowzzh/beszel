package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFanSpeedsWithWhitelist(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon2")
	require.NoError(t, os.MkdirAll(hwmon, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "name"), []byte("it8613\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "fan2_input"), []byte("1132\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "fan3_input"), []byte("9999\n"), 0o644))

	config := newFanConfigWithEnv(root, "it8613_fan2,it8613_fan3", false)
	assert.Equal(t, map[string]float64{
		"it8613_fan2": 1132,
		"it8613_fan3": 9999,
	}, readFanSpeeds(config))
}

func TestReadFanSpeedsSkipsInvalidValues(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	require.NoError(t, os.MkdirAll(hwmon, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "name"), []byte("testchip\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "fan1_input"), []byte("0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(hwmon, "fan2_input"), []byte("invalid\n"), 0o644))

	assert.Empty(t, readFanSpeeds(newFanConfigWithEnv(root, "", false)))
}
