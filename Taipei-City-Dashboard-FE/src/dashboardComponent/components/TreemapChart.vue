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

// 單色階模式：當 chart_config.color 只有 1 色，且 chart_config.colorRamp 提供 N 階 ramp，
// 用 colorScale.ranges 自訂 N 階（小值對應 ramp[0]、大值對應 ramp[last]），
// 比 ApexCharts 內建 shadeIntensity 對小範圍資料更明顯。多色或無 ramp 維持原行為。
const isSingleColor = props.chart_config.color?.length === 1;
const ramp = Array.isArray(props.chart_config.colorRamp) && props.chart_config.colorRamp.length >= 2
	? props.chart_config.colorRamp
	: null;

function buildSingleColorRanges(series) {
	if (!isSingleColor || !ramp) return null;
	const yValues = (series || []).flatMap((s) =>
		Array.isArray(s?.data)
			? s.data.map((d) => d?.y).filter((v) => typeof v === "number" && !Number.isNaN(v))
			: [],
	);
	if (yValues.length < 2) return null;
	const min = Math.min(...yValues);
	const max = Math.max(...yValues);
	if (min === max) return null; // 全同值無法分階
	const N = ramp.length;
	const step = (max - min) / N;
	return ramp.map((color, i) => ({
		from: min + step * i,
		to: i === N - 1 ? max + 1 : min + step * (i + 1),
		color,
	}));
}

const colorScaleRanges = buildSingleColorRanges(props.series);

const chartOptions = ref({
	chart: {
		borderRadius: 5,
		toolbar: {
			show: false,
		},
	},
	colors: [...props.chart_config.color],
	dataLabels: {
		formatter: function (val, { dataPointIndex }) {
			// 單色階模式所有格子都顯示名字；多色模式只顯示前 6 名避免擁擠
			if (isSingleColor) return val;
			return dataPointIndex > 5 ? "" : val;
		},
	},
	grid: {
		show: false,
	},
	legend: {
		show: false,
	},
	plotOptions: {
		treemap: {
			distributed: !isSingleColor,
			// 有 colorScale.ranges 時關 enableShades 避免雙重 shading；無 ranges 時 fallback 到 enableShades
			enableShades: isSingleColor && !colorScaleRanges,
			shadeIntensity: isSingleColor && !colorScaleRanges ? 1 : 0,
			reverseNegativeShade: false,
			...(colorScaleRanges
				? { colorScale: { ranges: colorScaleRanges } }
				: {}),
		},
	},
	stroke: {
		colors: [
			(typeof window !== "undefined" && window.getComputedStyle
				? window.getComputedStyle(document.body).getPropertyValue("--color-component-background").trim()
				: "") || "#282a2c",
		],
		show: true,
		width: 2,
	},
	tooltip: {
		custom: function ({
			series,
			seriesIndex,
			dataPointIndex,
			w,
		}) {
			// The class "chart-tooltip" could be edited in /assets/styles/chartStyles.css
			return (
				'<div class="chart-tooltip">' +
				"<h6>" +
				w.globals.categoryLabels[dataPointIndex] +
				"</h6>" +
				"<span>" +
				series[seriesIndex][dataPointIndex] +
				` ${props.chart_config.unit}` +
				"</span>" +
				"</div>"
			);
		},
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
});

const sum = computed(() => {
	let sum = 0;
	props.series[0].data.forEach(
		(item) => (sum += item.y)
	);
	return Math.round(sum * 100) / 100;
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
				config.w.globals.categoryLabels[config.dataPointIndex],
				null
			);
		}
		// Supports filtering by xAxis
		else if (props.map_filter.mode === "byLayer") {
			emits(
				"filterByLayer",
				props.map_config,
				config.w.globals.categoryLabels[config.dataPointIndex]
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
  <div
    v-if="activeChart === 'TreemapChart'"
    class="treemapchart"
  >
    <div class="treemapchart-title">
      <h5>總合</h5>
      <h6>{{ chart_config.total_label || `${sum} ${chart_config.unit}` }}</h6>
    </div>
    <VueApexCharts
      width="100%"
      type="treemap"
      :options="chartOptions"
      :series="series"
      @data-point-selection="handleDataSelection"
    />
  </div>
</template>

<style scoped lang="scss">
.treemapchart {
	&-title {
		display: flex;
		justify-content: center;
		flex-direction: column;
		margin: 0.5rem 0 -0.5rem;

		h5 {
			margin: 0;
			color: var(--color-complement-text);
		}

		h6 {
			margin: 0;
			color: var(--color-normal-text);
			font-size: var(--font-l);
			font-weight: 400;
		}
	}

}
</style>
