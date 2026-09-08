//go:build darwin

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePowermetricsData(t *testing.T) {
	input := `
Machine model: Mac14,10
OS version: 25D125

*** Sampled system activity (Sat Feb 14 00:42:06 2026 -0500) (503.05ms elapsed) ***

**** GPU usage ****

GPU HW active frequency: 444 MHz
GPU HW active residency:   0.97% (444 MHz: .97% 612 MHz:   0% 808 MHz:   0% 968 MHz:   0% 1110 MHz:   0% 1236 MHz:   0% 1338 MHz:   0% 1398 MHz:   0%)
GPU SW requested state: (P1 : 100% P2 :   0% P3 :   0% P4 :   0% P5 :   0% P6 :   0% P7 :   0% P8 :   0%)
GPU idle residency:  99.03%
GPU Power: 4 mW
`
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}
	valid := gm.parsePowermetricsData([]byte(input))
	require.True(t, valid)

	g0, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", g0.Name)
	// Usage = 100 - 99.03 = 0.97
	assert.InDelta(t, 0.97, g0.Usage, 0.01)
	// 4 mW -> 0.004 W
	assert.InDelta(t, 0.004, g0.Power, 0.0001)
	assert.Equal(t, 1.0, g0.Count)
}

func TestParsePowermetricsDataPartial(t *testing.T) {
	// Only power line (e.g. older macOS or different sampler output)
	input := `
**** GPU usage ****
GPU Power: 120 mW
`
	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}
	valid := gm.parsePowermetricsData([]byte(input))
	require.True(t, valid)

	g0, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", g0.Name)
	assert.InDelta(t, 0.12, g0.Power, 0.001)
	assert.Equal(t, 1.0, g0.Count)
}

func TestParseMacmonLine(t *testing.T) {
	input := `{"all_power":0.6468324661254883,"ane_power":0.0,"cpu_power":0.6359732151031494,"ecpu_usage":[2061,0.1726151406764984],"fans":[{"name":"fan0","rpm":999,"max_rpm":4900}],"gpu_power":0.010859241709113121,"gpu_ram_power":0.000965250947047025,"gpu_usage":[503,0.013633215799927711],"memory":{"ram_total":17179869184,"ram_usage":12322914304,"swap_total":0,"swap_usage":0},"pcpu_usage":[1248,0.11792058497667313],"ram_power":0.14885640144348145,"sys_power":10.4955415725708,"temp":{"cpu_temp_avg":23.041261672973633,"gpu_temp_avg":29.44516944885254},"timestamp":"2026-02-17T19:34:27.942556+00:00"}`

	gm := &GPUManager{
		GpuDataMap: make(map[string]*system.GPUData),
	}
	valid := gm.parseMacmonLine([]byte(input))
	require.True(t, valid)

	g0, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Apple GPU", g0.Name)
	// macmon reports usage fraction 0..1; expect percent conversion.
	assert.InDelta(t, 1.3633, g0.Usage, 0.05)
	// power includes gpu_power + gpu_ram_power
	assert.InDelta(t, 0.011824, g0.Power, 0.0005)
	assert.InDelta(t, 29.445, g0.Temperature, 0.01)
	assert.Equal(t, 1.0, g0.Count)

	appleStats := gm.getAppleSystemStats(60_000)
	assert.Equal(t, 0.64, appleStats.cpuPower)
	assert.InDelta(t, 23.041, appleStats.cpuTemperature, 0.01)
	assert.Equal(t, map[string]float64{"macmon_fan0": 999}, appleStats.fans)
}

func TestMacmonCpuPowerUsesNewSamplesPerCache(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	require.True(t, gm.parseMacmonLine([]byte(`{"cpu_power":2,"temp":{"cpu_temp_avg":40}}`)))
	require.True(t, gm.parseMacmonLine([]byte(`{"cpu_power":4,"temp":{"cpu_temp_avg":42}}`)))

	first := gm.getAppleSystemStats(60_000)
	assert.Equal(t, 3.0, first.cpuPower)
	assert.Equal(t, 42.0, first.cpuTemperature)

	require.True(t, gm.parseMacmonLine([]byte(`{"cpu_power":9,"temp":{"cpu_temp_avg":45}}`)))
	second := gm.getAppleSystemStats(60_000)
	assert.Equal(t, 9.0, second.cpuPower)
	assert.Equal(t, 45.0, second.cpuTemperature)

	unchanged := gm.getAppleSystemStats(60_000)
	assert.Equal(t, 9.0, unchanged.cpuPower)
}

func TestParseMacmonLineWithoutGpuMetrics(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	valid := gm.parseMacmonLine([]byte(`{"cpu_power":1.25,"fans":[{"name":"fan0","rpm":1200}],"temp":{"cpu_temp_avg":42.5}}`))
	require.True(t, valid)
	assert.Empty(t, gm.GpuDataMap)

	appleStats := gm.getAppleSystemStats(60_000)
	assert.Equal(t, 1.25, appleStats.cpuPower)
	assert.Equal(t, 42.5, appleStats.cpuTemperature)
	assert.Equal(t, map[string]float64{"macmon_fan0": 1200}, appleStats.fans)
}

func TestParseMacmonLinePreservesStoppedFan(t *testing.T) {
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	valid := gm.parseMacmonLine([]byte(`{"fans":[{"name":"fan0","rpm":0}]}`))
	require.True(t, valid)

	appleStats := gm.getAppleSystemStats(60_000)
	assert.Equal(t, map[string]float64{"macmon_fan0": 0}, appleStats.fans)
}
