package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

type FanConfig struct {
	root           string
	fans           map[string]struct{}
	isBlacklist    bool
	hasWildcards   bool
	skipCollection bool
}

func newFanConfig() *FanConfig {
	sysSensors, _ := utils.GetEnv("SYS_SENSORS")
	fansEnvVal, fansSet := utils.GetEnv("FANS")
	return newFanConfigWithEnv(sysSensors, fansEnvVal, fansSet && fansEnvVal == "")
}

func newFanConfigWithEnv(sysSensors, fansEnvVal string, skipCollection bool) *FanConfig {
	config := &FanConfig{
		root:           "/sys",
		fans:           make(map[string]struct{}),
		skipCollection: skipCollection,
	}
	if sysSensors != "" {
		config.root = sysSensors
	}
	if strings.HasPrefix(fansEnvVal, "-") {
		config.isBlacklist = true
		fansEnvVal = fansEnvVal[1:]
	}
	for fan := range strings.SplitSeq(fansEnvVal, ",") {
		fan = strings.TrimSpace(fan)
		if fan == "" {
			continue
		}
		config.fans[fan] = struct{}{}
		if strings.Contains(fan, "*") {
			config.hasWildcards = true
		}
	}
	return config
}

func (a *Agent) updateFans(systemStats *system.Stats) {
	if a.fanConfig == nil || a.fanConfig.skipCollection {
		return
	}
	if fans := readFanSpeeds(a.fanConfig); len(fans) > 0 {
		systemStats.Fans = fans
	}
}

func readFanSpeeds(config *FanConfig) map[string]float64 {
	hwmonDirs, _ := filepath.Glob(filepath.Join(config.root, "class", "hwmon", "hwmon*"))
	fans := make(map[string]float64)
	for _, hwmonDir := range hwmonDirs {
		chipBytes, err := os.ReadFile(filepath.Join(hwmonDir, "name"))
		if err != nil {
			continue
		}
		chip := strings.TrimSpace(string(chipBytes))
		if chip == "" {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(hwmonDir, "fan*_input"))
		for _, input := range inputs {
			fileName := filepath.Base(input)
			index := strings.TrimSuffix(strings.TrimPrefix(fileName, "fan"), "_input")
			if _, err := strconv.ParseUint(index, 10, 16); err != nil {
				continue
			}
			key := chip + "_fan" + index
			if !isValidFan(key, config) {
				continue
			}
			rpmBytes, err := os.ReadFile(input)
			if err != nil {
				continue
			}
			rpm, err := strconv.ParseFloat(strings.TrimSpace(string(rpmBytes)), 64)
			if err != nil || rpm <= 0 || rpm > 100000 {
				continue
			}
			fans[key] = rpm
		}
	}
	return fans
}

func isValidFan(fanName string, config *FanConfig) bool {
	return isValidMetric(fanName, config.fans, config.isBlacklist, config.hasWildcards)
}
