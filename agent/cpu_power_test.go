package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writePowercapValue(t *testing.T, path string, value string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(value), 0o644))
}

func createPowercapZone(t *testing.T, root, dir, name, energy, maxEnergy string) string {
	t.Helper()
	zonePath := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(zonePath, 0o755))
	writePowercapValue(t, filepath.Join(zonePath, "name"), name)
	writePowercapValue(t, filepath.Join(zonePath, "energy_uj"), energy)
	writePowercapValue(t, filepath.Join(zonePath, "max_energy_range_uj"), maxEnergy)
	return zonePath
}

func TestCpuPowerMonitorCalculatesPackageWatts(t *testing.T) {
	root := t.TempDir()
	zonePath := createPowercapZone(t, root, "intel-rapl:0", "package-0", "1000000", "100000000")
	createPowercapZone(t, root, "intel-rapl:0:0", "core", "900000", "100000000")

	now := time.Unix(1_000, 0)
	monitor, err := newCpuPowerMonitor(root, func() time.Time { return now })
	require.NoError(t, err)
	require.Len(t, monitor.zones, 1)

	now = now.Add(2 * time.Second)
	writePowercapValue(t, filepath.Join(zonePath, "energy_uj"), "7000000")
	watts, ok := monitor.getWatts(defaultDataCacheTimeMs)
	require.True(t, ok)
	require.Equal(t, 3.0, watts)
}

func TestCpuPowerMonitorHandlesCounterWrap(t *testing.T) {
	root := t.TempDir()
	zonePath := createPowercapZone(t, root, "intel-rapl:0", "package-0", "9000000", "10000000")

	now := time.Unix(2_000, 0)
	monitor, err := newCpuPowerMonitor(root, func() time.Time { return now })
	require.NoError(t, err)

	now = now.Add(time.Second)
	writePowercapValue(t, filepath.Join(zonePath, "energy_uj"), "1000000")
	watts, ok := monitor.getWatts(defaultDataCacheTimeMs)
	require.True(t, ok)
	require.Equal(t, 2.0, watts)
}

func TestCpuPowerMonitorNeedsReadablePackageZone(t *testing.T) {
	root := t.TempDir()
	createPowercapZone(t, root, "intel-rapl:0:0", "core", "1000000", "10000000")

	_, err := newCpuPowerMonitor(root, time.Now)
	require.Error(t, err)
}
