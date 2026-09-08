import { t } from "@lingui/core/macro"
import AreaChartDefault from "@/components/charts/area-chart"
import { batteryStateTranslations } from "@/lib/i18n"
import { $temperatureFilter, $userSettings } from "@/lib/stores"
import { cn, decimalString, formatTemperature, toFixedFloat } from "@/lib/utils"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard, FilterBar, SelectAvgMax } from "../chart-card"
import LineChartDefault from "@/components/charts/line-chart"
import { useStore } from "@nanostores/react"
import { useRef, useMemo, useState, useEffect } from "react"

const sensorDisplayNames: Record<string, string> = {
	"AMD GPU (13c0)": "核显温度",
	"Apple GPU": "GPU 温度",
	"NAND CH0 temp": "SSD 温度",
	amdgpu_edge: "核显边缘温度",
	it8613: "主板温度 1",
	it8613_5: "主板温度 2",
	it8613_6: "主板温度 3",
	k10temp_tccd1: "CPU 核心温度",
	k10temp_tctl: "CPU 温度",
	macmon_cpu: "CPU 温度",
	"r8169_0_400:00": "有线网卡温度",
	nvme_composite: "SSD 综合温度",
	nvme_sensor_1: "SSD 传感器 1",
	nvme_sensor_2: "SSD 传感器 2",
	spd5118: "内存温度 1",
	spd5118_11: "内存温度 2",
}

function getSensorDisplayName(sensorName: string) {
	return sensorDisplayNames[sensorName] ?? sensorName
}

const fanDisplayNames: Record<string, string> = {
	it8613_fan2: "风扇 2",
	it8613_fan3: "涡轮风扇",
	macmon_fan0: "系统风扇",
}

export function FanSpeedChart({
	chartData,
	grid,
	dataEmpty,
	showMax,
	isLongerChart,
	maxValues,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	showMax: boolean
	isLongerChart: boolean
	maxValues: boolean
}) {
	const fanNames = Array.from(new Set(chartData.systemStats.flatMap(({ stats }) => Object.keys(stats?.f ?? {})))).sort()

	if (fanNames.length === 0) {
		return null
	}

	const maxValSelect = isLongerChart ? <SelectAvgMax max={maxValues} /> : null
	const dataPoints = fanNames.map((fanName, index) => ({
		label: fanDisplayNames[fanName] ?? fanName,
		dataKey: ({ stats }: SystemStatsRecord) => (showMax ? stats?.fm?.[fanName] : stats?.f?.[fanName]),
		color: index + 1,
	}))

	return (
		<ChartCard empty={dataEmpty} grid={grid} title="风扇转速" description="硬件风扇转速" cornerEl={maxValSelect}>
			<LineChartDefault
				chartData={chartData}
				maxToggled={showMax}
				dataPoints={dataPoints}
				tickFormatter={(val) => `${toFixedFloat(val, 0)} RPM`}
				contentFormatter={({ value }) => `${decimalString(value)} RPM`}
			/>
		</ChartCard>
	)
}

export function BatteryChart({
	chartData,
	grid,
	dataEmpty,
	maxValues,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	maxValues: boolean
}) {
	const showBatteryChart = chartData.systemStats.at(-1)?.stats.bat

	if (!showBatteryChart) {
		return null
	}

	return (
		<ChartCard
			empty={dataEmpty}
			grid={grid}
			title={t`Battery`}
			description={`${t({
				message: "Current state",
				comment: "Context: Battery state",
			})}: ${batteryStateTranslations[chartData.systemStats.at(-1)?.stats.bat?.[1] ?? 0]()}`}
		>
			<AreaChartDefault
				chartData={chartData}
				maxToggled={maxValues}
				dataPoints={[
					{
						label: t`Charge`,
						dataKey: ({ stats }) => stats?.bat?.[0],
						color: 1,
						opacity: 0.35,
					},
				]}
				domain={[0, 100]}
				tickFormatter={(val) => `${val}%`}
				contentFormatter={({ value }) => `${value}%`}
			/>
		</ChartCard>
	)
}

export function TemperatureChart({
	chartData,
	grid,
	dataEmpty,
	setPageBottomExtraMargin,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
	setPageBottomExtraMargin?: (margin: number) => void
}) {
	const showTempChart = chartData.systemStats.at(-1)?.stats.t

	const filter = useStore($temperatureFilter)
	const userSettings = useStore($userSettings)

	const statsRef = useRef(chartData.systemStats)
	statsRef.current = chartData.systemStats

	// Derive sensor names key from latest data point
	let sensorNamesKey = ""
	for (let i = chartData.systemStats.length - 1; i >= 0; i--) {
		const t = chartData.systemStats[i].stats?.t
		if (t) {
			sensorNamesKey = Object.keys(t).sort().join("\0")
			break
		}
	}

	// Only recompute colors and dataKey functions when sensor names change
	const { colorMap, dataKeys, sortedKeys } = useMemo(() => {
		const stats = statsRef.current
		const tempSums = {} as Record<string, number>
		for (const data of stats) {
			const t = data.stats?.t
			if (!t) continue
			for (const key of Object.keys(t)) {
				tempSums[key] = (tempSums[key] ?? 0) + t[key]
			}
		}
		const sorted = Object.keys(tempSums).sort((a, b) => tempSums[b] - tempSums[a])
		const colorMap = {} as Record<string, string>
		const dataKeys = {} as Record<string, (d: SystemStatsRecord) => number | undefined>
		for (let i = 0; i < sorted.length; i++) {
			const key = sorted[i]
			colorMap[key] = `hsl(${((i * 360) / sorted.length) % 360}, 60%, 55%)`
			dataKeys[key] = (d: SystemStatsRecord) => d.stats?.t?.[key]
		}
		return { colorMap, dataKeys, sortedKeys: sorted }
	}, [sensorNamesKey])

	const dataPoints = useMemo(() => {
		return sortedKeys.map((key) => {
			const displayName = getSensorDisplayName(key)
			const filterTerms = filter
				? filter
						.toLowerCase()
						.split(" ")
						.filter((term) => term.length > 0)
				: []
			const searchableName = `${displayName} ${key}`.toLowerCase()
			const filtered = filterTerms.length > 0 && !filterTerms.some((term) => searchableName.includes(term))
			const strokeOpacity = filtered ? 0.1 : 1
			return {
				label: displayName,
				dataKey: dataKeys[key],
				color: colorMap[key],
				opacity: strokeOpacity,
			}
		})
	}, [sortedKeys, filter, dataKeys, colorMap])

	// test with lots of data points
	// const totalPoints = 50
	// if (dataPoints.length > 0 && dataPoints.length < totalPoints) {
	// 	let i = 0
	// 	while (dataPoints.length < totalPoints) {
	// 		dataPoints.push({
	// 			label: `Test ${++i}`,
	// 			dataKey: () => 0,
	// 			color: "red",
	// 			opacity: 1,
	// 		})
	// 	}
	// }

	const chartRef = useRef<HTMLDivElement>(null)
	const [addMargin, setAddMargin] = useState(false)
	const marginPx = (dataPoints.length - 13) * 18

	useEffect(() => {
		if (setPageBottomExtraMargin && dataPoints.length > 13 && chartRef.current) {
			const checkPosition = () => {
				if (!chartRef.current) return
				const rect = chartRef.current.getBoundingClientRect()
				const actualScrollHeight = addMargin
					? document.documentElement.scrollHeight - marginPx
					: document.documentElement.scrollHeight
				const distanceToBottom = actualScrollHeight - (rect.bottom + window.scrollY)

				if (distanceToBottom < 250) {
					setAddMargin(true)
					setPageBottomExtraMargin(marginPx)
				} else {
					setAddMargin(false)
					setPageBottomExtraMargin(0)
				}
			}
			checkPosition()
			const timer = setTimeout(checkPosition, 500)
			return () => {
				clearTimeout(timer)
			}
		} else if (addMargin) {
			setAddMargin(false)
			if (setPageBottomExtraMargin) setPageBottomExtraMargin(0)
		}
	}, [dataPoints.length, addMargin, marginPx, setPageBottomExtraMargin])

	if (!showTempChart) {
		return null
	}

	const legend = dataPoints.length < 12

	return (
		<div ref={chartRef} className={cn("odd:last-of-type:col-span-full", { "col-span-full": !grid })}>
			<ChartCard
				empty={dataEmpty}
				grid={grid}
				title={t`Temperature`}
				description={t`Temperatures of system sensors`}
				cornerEl={<FilterBar store={$temperatureFilter} />}
				legend={legend}
			>
				<LineChartDefault
					chartData={chartData}
					itemSorter={(a, b) => b.value - a.value}
					domain={["auto", "auto"]}
					legend={legend}
					tickFormatter={(val) => {
						const { value, unit } = formatTemperature(val, userSettings.unitTemp)
						return `${toFixedFloat(value, 2)} ${unit}`
					}}
					contentFormatter={(item) => {
						const { value, unit } = formatTemperature(item.value, userSettings.unitTemp)
						return `${decimalString(value)} ${unit}`
					}}
					dataPoints={dataPoints}
				></LineChartDefault>
			</ChartCard>
		</div>
	)
}
