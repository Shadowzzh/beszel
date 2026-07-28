//go:build testing

package records

import (
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// DeleteOldSystemStats exposes deleteOldSystemStats for testing
func DeleteOldSystemStats(app core.App) error {
	return deleteOldSystemStats(app)
}

// DeleteOldAlertsHistory exposes deleteOldAlertsHistory for testing
func DeleteOldAlertsHistory(app core.App, countToKeep, countBeforeDeletion int) error {
	return deleteOldAlertsHistory(app, countToKeep, countBeforeDeletion)
}

// TwoDecimals exposes twoDecimals for testing
func TwoDecimals(value float64) float64 {
	return twoDecimals(value)
}

// AverageSystemStatsForTest 让外部测试包验证历史聚合结果。
func AverageSystemStatsForTest(rm *RecordManager, db dbx.Builder, ids []string) *system.Stats {
	recordIds := make(RecordIds, len(ids))
	for i, id := range ids {
		recordIds[i].Id = id
	}
	return rm.AverageSystemStats(db, recordIds)
}
