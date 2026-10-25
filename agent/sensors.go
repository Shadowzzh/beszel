package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/sensors"
)

var errTemperatureFetchTimeout = errors.New("temperature collection timed out")

// Matches sensors.TemperaturesWithContext to allow for panic recovery (gopsutil/issues/1832)
type getTempsFn func(ctx context.Context) ([]sensors.TemperatureStat, error)

type SensorConfig struct {
	context context.Context
	sensors map[string]struct{}
	// temperatureOffsets applies a per-sensor correction (in Celsius) before values
	// are stored or used for the dashboard temperature. Useful for engineering
	// samples whose DTS calibration reads systematically low or high.
	temperatureOffsets map[string]float64
	primarySensor      string
	timeout            time.Duration
	isBlacklist        bool
	hasWildcards       bool
	skipCollection     bool
	firstRun           bool
}

func (a *Agent) newSensorConfig() *SensorConfig {
	primarySensor, _ := utils.GetEnv("PRIMARY_SENSOR")
	sysSensors, _ := utils.GetEnv("SYS_SENSORS")
	sensorsEnvVal, sensorsSet := utils.GetEnv("SENSORS")
	skipCollection := sensorsSet && sensorsEnvVal == ""
	sensorsTimeout, _ := utils.GetEnv("SENSORS_TIMEOUT")
	sensorOffsets, _ := utils.GetEnv("SENSOR_OFFSETS")

	config := a.newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal, sensorsTimeout, skipCollection)
	config.temperatureOffsets = parseSensorOffsets(sensorOffsets)
	return config
}

// newSensorConfigWithEnv creates a SensorConfig with the provided environment variables
// sensorsSet indicates if the SENSORS environment variable was explicitly set (even to empty string)
func (a *Agent) newSensorConfigWithEnv(primarySensor, sysSensors, sensorsEnvVal, sensorsTimeout string, skipCollection bool) *SensorConfig {
	timeout := 2 * time.Second
	if sensorsTimeout != "" {
		if d, err := time.ParseDuration(sensorsTimeout); err == nil {
			timeout = d
		} else {
			slog.Warn("Invalid SENSORS_TIMEOUT", "value", sensorsTimeout)
		}
	}

	config := &SensorConfig{
		context:            context.Background(),
		primarySensor:      primarySensor,
		timeout:            timeout,
		skipCollection:     skipCollection,
		firstRun:           true,
		sensors:            make(map[string]struct{}),
		temperatureOffsets: make(map[string]float64),
	}

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors != "" {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		config.context = context.WithValue(config.context,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	// handle blacklist
	if strings.HasPrefix(sensorsEnvVal, "-") {
		config.isBlacklist = true
		sensorsEnvVal = sensorsEnvVal[1:]
	}

	for sensor := range strings.SplitSeq(sensorsEnvVal, ",") {
		sensor = strings.TrimSpace(sensor)
		if sensor != "" {
			config.sensors[sensor] = struct{}{}
			if strings.Contains(sensor, "*") {
				config.hasWildcards = true
			}
		}
	}

	return config
}

// parseSensorOffsets parses a SENSOR_OFFSETS value such as
// "coretemp_package_id_0=7.5,coretemp_core_0=7.5" into a name -> delta map.
// Invalid entries are logged and skipped so a typo cannot stop temperature collection.
func parseSensorOffsets(raw string) map[string]float64 {
	offsets := make(map[string]float64)
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			slog.Warn("Invalid SENSOR_OFFSETS entry (missing '=')", "entry", entry)
			continue
		}
		name = strings.TrimSpace(name)
		offset, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if name == "" || err != nil {
			slog.Warn("Invalid SENSOR_OFFSETS entry", "entry", entry, "err", err)
			continue
		}
		offsets[name] = offset
	}
	return offsets
}
func (a *Agent) updateTemperatures(systemStats *system.Stats) {
	// skip if sensors whitelist is set to empty string
	if a.sensorConfig.skipCollection {
		slog.Debug("Skipping temperature collection")
		return
	}

	// reset high temp
	a.systemInfo.DashboardTemp = 0

	temps, err := a.getTempsWithTimeout(getSensorTemps)
	if err != nil {
		// retry once on panic (gopsutil/issues/1832)
		if !errors.Is(err, errTemperatureFetchTimeout) {
			temps, err = a.getTempsWithTimeout(getSensorTemps)
		}
		if err != nil {
			slog.Warn("Error updating temperatures", "err", err)
			if len(systemStats.Temperatures) > 0 {
				systemStats.Temperatures = make(map[string]float64)
			}
			return
		}
	}
	slog.Debug("Temperature", "sensors", temps)

	// return if no sensors
	if len(temps) == 0 {
		return
	}

	systemStats.Temperatures = make(map[string]float64, len(temps))
	for i, sensor := range temps {
		// check for malformed strings on darwin (gopsutil/issues/1832)
		if runtime.GOOS == "darwin" && !utf8.ValidString(sensor.SensorKey) {
			continue
		}

		// scale temperature
		if sensor.Temperature != 0 && sensor.Temperature < 1 {
			sensor.Temperature = scaleTemperature(sensor.Temperature)
		}
		// skip if temperature is unreasonable
		if sensor.Temperature <= 0 || sensor.Temperature >= 200 {
			continue
		}
		sensorName := sensor.SensorKey
		if _, ok := systemStats.Temperatures[sensorName]; ok {
			// if key already exists, append int to key
			sensorName = sensorName + "_" + strconv.Itoa(i)
		}
		// skip if not in whitelist or blacklist
		if !isValidSensor(sensorName, a.sensorConfig) {
			continue
		}
		// apply per-sensor calibration offset (e.g. engineering sample DTS bias)
		if offset, ok := a.sensorConfig.temperatureOffsets[sensorName]; ok {
			sensor.Temperature += offset
		}
		// set dashboard temperature
		switch a.sensorConfig.primarySensor {
		case "":
			a.systemInfo.DashboardTemp = max(a.systemInfo.DashboardTemp, sensor.Temperature)
		case sensorName:
			a.systemInfo.DashboardTemp = sensor.Temperature
		}
		systemStats.Temperatures[sensorName] = utils.TwoDecimals(sensor.Temperature)
	}
}

// getTempsWithPanicRecovery wraps sensors.TemperaturesWithContext to recover from panics (gopsutil/issues/1832)
func (a *Agent) getTempsWithPanicRecovery(getTemps getTempsFn) (temps []sensors.TemperatureStat, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	// get sensor data (error ignored intentionally as it may be only with one sensor)
	temps, _ = getTemps(a.sensorConfig.context)
	return
}

func (a *Agent) getTempsWithTimeout(getTemps getTempsFn) ([]sensors.TemperatureStat, error) {
	type result struct {
		temps []sensors.TemperatureStat
		err   error
	}

	// Use a longer timeout on the first run to allow for initialization
	// (e.g. Windows LHM subprocess startup)
	timeout := a.sensorConfig.timeout
	if a.sensorConfig.firstRun {
		a.sensorConfig.firstRun = false
		timeout = 10 * time.Second
	}

	resultCh := make(chan result, 1)
	go func() {
		temps, err := a.getTempsWithPanicRecovery(getTemps)
		resultCh <- result{temps: temps, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.temps, res.err
	case <-time.After(timeout):
		return nil, errTemperatureFetchTimeout
	}
}

// isValidSensor checks if a sensor is valid based on the sensor name and the sensor config
func isValidSensor(sensorName string, config *SensorConfig) bool {
	return isValidMetric(sensorName, config.sensors, config.isBlacklist, config.hasWildcards)
}

func isValidMetric(metricName string, configured map[string]struct{}, isBlacklist, hasWildcards bool) bool {
	// if no sensors configured, everything is valid
	if len(configured) == 0 {
		return true
	}

	// Exact match - return true if whitelist, false if blacklist
	if _, exactMatch := configured[metricName]; exactMatch {
		return !isBlacklist
	}

	// If no wildcards, return true if blacklist, false if whitelist
	if !hasWildcards {
		return isBlacklist
	}

	// Check for wildcard patterns
	for pattern := range configured {
		if !strings.Contains(pattern, "*") {
			continue
		}
		if match, _ := path.Match(pattern, metricName); match {
			return !isBlacklist
		}
	}

	return isBlacklist
}

// scaleTemperature scales temperatures in fractional values to reasonable Celsius values
func scaleTemperature(temp float64) float64 {
	if temp > 1 {
		return temp
	}
	scaled100 := temp * 100
	scaled1000 := temp * 1000

	if scaled100 >= 15 && scaled100 <= 95 {
		return scaled100
	} else if scaled1000 >= 15 && scaled1000 <= 95 {
		return scaled1000
	}
	return scaled100
}
