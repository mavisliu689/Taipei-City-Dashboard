<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->
<script setup>
import { ref, computed } from "vue";
import VueApexCharts from "vue3-apexcharts";

const props = defineProps([
	"chart_config",
	"activeChart",
	"series",
	"map_config",
	"map_filter",
	"map_filter_on",
]);

const emits = defineEmits([
	"filterByParam",
	"filterByLayer",
	"clearByParamFilter",
	"clearByLayerFilter",
	"fly"
]);

// 單色階模式：當 chart_config.color 只有 1 色 且 chart_config.colorRamp 是 N 階陣列，
// 從 ramp 等分取「條數」個顏色（top N 是大→小排序，所以淺色給大值、深色給小值）。
// 多色或無 ramp 維持原 colors（每條依 distributed 循環同 array）。
const isSingleColor = props.chart_config.color?.length === 1;
const ramp = Array.isArray(props.chart_config.colorRamp) && props.chart_config.colorRamp.length >= 2
	? props.chart_config.colorRamp
	: null;

function buildBarColors(series) {
	if (!isSingleColor || !ramp) return [...props.chart_config.color];
	const {length} = series?.[0]?.data || [];
	if (length === 0) return [...props.chart_config.color];
	if (length === 1) return [ramp[ramp.length - 1]]; // 只 1 條時用最淺
	// i=0（top 1，最大）→ ramp[last] 淺；i=last（top N，最小）→ ramp[0] 深
	return Array.from({ length }, (_, i) => {
		const t = i / (length - 1);
		const idx = Math.round((1 - t) * (ramp.length - 1));
		return ramp[idx];
	});
}

const chartOptions = ref({
	chart: {
		// offsetY 原本 +15 會在 chart 上方留 SVG 空白（給 vertical bar dataLabel 用），horizontal bar 不需要
		stacked: true,
		toolbar: {
			show: false,
		},
	},
	colors: buildBarColors(props.series),
	dataLabels: {
		// position=top（外部）時靠近條柱、不要太遠避免頂到卡片右邊；center 走原 +20
		offsetX: props.chart_config.showDataLabels === true ? 16 : 20,
		textAnchor: "start",
	},
	grid: {
		show: false,
		// showDataLabels=true 時右側留更多空間，避免 3 位數緊貼條柱或貼齊卡片邊緣
		...(props.chart_config.showDataLabels === true ? { padding: { right: 48 } } : {}),
	},
	legend: {
		show: false,
	},
	plotOptions: {
		bar: {
			borderRadius: 5,
			borderRadiusApplication: "end", // 只圓化右端（橫向 bar 的 end），跟 ColumnChart 一致圓頭
			barHeight: "40%", // 條柱細程度
			distributed: true,
			horizontal: true,
			dataLabels: {
				hideOverflowingLabels: false,
				// showDataLabels=true 時數字到條柱右端外側（橫向 bar 的 'top' = 右端）；
				// 未設或 false 維持 center（條柱內部）
				position: props.chart_config.showDataLabels === true ? "top" : "center",
			},
		},
	},
	stroke: {
		colors: ["#282a2c"],
		show: true,
		width: 0,
	},
	// The class "chart-tooltip" could be edited in /assets/styles/chartStyles.css
	tooltip: {
		custom: function ({
			series,
			seriesIndex,
			dataPointIndex,
			w,
		}) {
			return (
				'<div class="chart-tooltip">' +
				"<h6>" +
				w.globals.labels[dataPointIndex] +
				"</h6>" +
				"<span>" +
				series[seriesIndex][dataPointIndex] +
				` ${props.chart_config.unit}` +
				"</span>" +
				"</div>"
			);
		},
		followCursor: true,
	},
	xaxis: {
		axisBorder: {
			show: false,
		},
		axisTicks: {
			show: false,
		},
		labels: {
			show: false,
		},
		type: "category",
	},
	yaxis: {
		labels: {
			formatter: function (value) {
				return value.length > 7 ? value.slice(0, 6) + "..." : value;
			},
		},
	},
});

const chartHeight = computed(() => {
	return `${40 + props.series[0].data.length * 30}`;
});

const selectedIndex = ref(null);

function handleDataSelection(_e, _chartContext, config) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}
	if (
		`${config.dataPointIndex}-${config.seriesIndex}` !== selectedIndex.value
	) {
		// Supports filtering by xAxis
		if (props.map_filter.mode === "byParam") {
			emits(
				"filterByParam",
				props.map_filter,
				props.map_config,
				config.w.globals.labels[config.dataPointIndex],
				null
			);
		}
		// Supports filtering by xAxis
		else if (props.map_filter.mode === "byLayer") {
			emits(
				"filterByLayer",
				props.map_config,
				config.w.globals.labels[config.dataPointIndex]
			);
		}
		selectedIndex.value = `${config.dataPointIndex}-${config.seriesIndex}`;
	} else {
		if (props.map_filter.mode === "byParam") {
			emits("clearByParamFilter", props.map_config);
		} else if (props.map_filter.mode === "byLayer") {
			emits("clearByLayerFilter", props.map_config);
		}
		selectedIndex.value = null;
	}
}
</script>

<template>
  <div v-if="activeChart === 'BarChart'">
    <VueApexCharts
      width="100%"
      :height="chartHeight"
      type="bar"
      :options="chartOptions"
      :series="series"
      @data-point-selection="handleDataSelection"
    />
  </div>
</template>
