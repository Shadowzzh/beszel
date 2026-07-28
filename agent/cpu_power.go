package agent

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

const defaultPowercapRoot = "/sys/class/powercap"

type cpuPowerZone struct {
	energyPath     string
	maxEnergyRange uint64
}

type cpuPowerSample struct {
	energy []uint64
	time   time.Time
}

type cpuPowerMonitor struct {
	zones   []cpuPowerZone
	last    map[uint16]cpuPowerSample
	nowFunc func() time.Time
}

func newCpuPowerMonitor(root string, nowFunc func() time.Time) (*cpuPowerMonitor, error) {
	namePaths, err := filepath.Glob(filepath.Join(root, "*", "name"))
	if err != nil {
		return nil, err
	}

	monitor := &cpuPowerMonitor{
		last:    make(map[uint16]cpuPowerSample),
		nowFunc: nowFunc,
	}
	for _, namePath := range namePaths {
		name, ok := utils.ReadStringFileOK(namePath)
		if !ok || !strings.HasPrefix(name, "package-") {
			continue
		}
		zonePath := filepath.Dir(namePath)
		energyPath := filepath.Join(zonePath, "energy_uj")
		maxEnergyPath := filepath.Join(zonePath, "max_energy_range_uj")
		if _, ok := utils.ReadUintFile(energyPath); !ok {
			continue
		}
		maxEnergyRange, ok := utils.ReadUintFile(maxEnergyPath)
		if !ok || maxEnergyRange == 0 {
			continue
		}
		monitor.zones = append(monitor.zones, cpuPowerZone{
			energyPath:     energyPath,
			maxEnergyRange: maxEnergyRange,
		})
	}
	if len(monitor.zones) == 0 {
		return nil, errors.New("no readable CPU package powercap zones")
	}

	initial, err := monitor.readSample()
	if err != nil {
		return nil, err
	}
	monitor.last[defaultDataCacheTimeMs] = initial
	return monitor, nil
}

func (m *cpuPowerMonitor) readSample() (cpuPowerSample, error) {
	sample := cpuPowerSample{
		energy: make([]uint64, len(m.zones)),
		time:   m.nowFunc(),
	}
	for i, zone := range m.zones {
		energy, ok := utils.ReadUintFile(zone.energyPath)
		if !ok {
			return cpuPowerSample{}, errors.New("failed to read CPU package energy")
		}
		sample.energy[i] = energy
	}
	return sample, nil
}

func (m *cpuPowerMonitor) getWatts(cacheTimeMs uint16) (float64, bool) {
	current, err := m.readSample()
	if err != nil {
		return 0, false
	}
	previous, ok := m.last[cacheTimeMs]
	m.last[cacheTimeMs] = current
	if !ok {
		return 0, false
	}

	elapsedSeconds := current.time.Sub(previous.time).Seconds()
	if elapsedSeconds <= 0 || len(previous.energy) != len(current.energy) {
		return 0, false
	}

	var energyDelta uint64
	for i, currentEnergy := range current.energy {
		previousEnergy := previous.energy[i]
		if currentEnergy >= previousEnergy {
			energyDelta += currentEnergy - previousEnergy
			continue
		}
		maxEnergyRange := m.zones[i].maxEnergyRange
		if previousEnergy > maxEnergyRange || currentEnergy > maxEnergyRange {
			return 0, false
		}
		energyDelta += maxEnergyRange - previousEnergy + currentEnergy
	}

	watts := float64(energyDelta) / 1_000_000 / elapsedSeconds
	if watts <= 0 || watts > 10_000 || math.IsNaN(watts) || math.IsInf(watts, 0) {
		return 0, false
	}
	return utils.TwoDecimals(watts), true
}
