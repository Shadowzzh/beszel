//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestAverageSystemStatsFanSpeed(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	user, err := tests.CreateUser(hub, "fan-speed@example.com", "testtesttest")
	require.NoError(t, err)
	systemRecord, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "fan-speed-test",
		"host":  "127.0.0.1",
		"port":  "45876",
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	first, err := tests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  `{"cpu":20,"f":{"it8613_fan2":1100}}`,
	})
	require.NoError(t, err)
	second, err := tests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  `{"cpu":40,"f":{"it8613_fan2":1300},"fm":{"it8613_fan2":1350}}`,
	})
	require.NoError(t, err)
	missing, err := tests.CreateRecord(hub, "system_stats", map[string]any{
		"system": systemRecord.Id,
		"type":   "1m",
		"stats":  `{"cpu":60}`,
	})
	require.NoError(t, err)

	stats := records.AverageSystemStatsForTest(
		records.NewRecordManager(hub),
		hub.DB(),
		[]string{first.Id, second.Id, missing.Id},
	)
	require.Equal(t, 1200.0, stats.Fans["it8613_fan2"])
	require.Equal(t, 1350.0, stats.MaxFans["it8613_fan2"])
}
