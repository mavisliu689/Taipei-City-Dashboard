<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->

<script setup>
import { computed, ref, watch } from "vue";
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

const isLargeDataSet = computed(() => {
	return props.series[0].data.length > 12
})

// Calculate initial width for large datasets only
const initialWidth = computed(() => {
	const WIDTH_PER_ITEM = 32
	const itemCount = props.series[0].data.length;
	return itemCount * WIDTH_PER_ITEM;
});

const widthValue = ref(initialWidth.value);

// Convert to a string with unit for ApexCharts
const chartWidth = computed(() => {
	return isLargeDataSet.value ? `${widthValue.value}px` : "100%";
});


// 平均值只顯示右上角文字，不畫 chart annotation 線。
// 切 dropdown 後 chart_config.average_line 被 mutation patch 時會即時更新，
// 不依賴 setup 階段的一次性 snapshot（避免 dropdown 切換 → :key remount → setup 拿舊 chart_config 的 race）
const averageLine = computed(() => props.chart_config.average_line);
const avgLineColor = computed(
	() => averageLine.value?.color || props.chart_config.color?.[0] || "#888",
);
const avgLineLabel = computed(() => {
	const al = averageLine.value;
	if (!al || !(al.value > 0)) return "";
	return al.label || `平均 ${al.value}`;
});
const showAvgCorner = computed(() => !!(averageLine.value && averageLine.value.value > 0));
const initialAverageLineAnnotations = [];

// ApexCharts API 即時更新 annotation（chartOptions.events 引用 captureChartCtx，必須先宣告）
let apexChartCtx = null;
const captureChartCtx = (ctx) => { apexChartCtx = ctx; };

function applyAverageAnnotation(al) {
	if (!apexChartCtx) return;
	apexChartCtx.updateOptions({
		annotations: {
			yaxis: [],
		},
	});
}

const chartOptions = ref({
	chart: {
		stacked: true,
		zoom: {
			allowMouseWheelZoom: false,
		},
		toolbar: isLargeDataSet.value
			? {
				show: true,
				tools: {
					download: false,
					pan: false,
					reset: "<p>" + "重置" + "</p>",
					zoomin: false,
					zoomout: false,
				}
			  }
			: {
				show: false,
			},
		events: {
			mounted: captureChartCtx, // chart 渲染完取 ApexCharts context，給 watch 用
			updated: captureChartCtx, // ApexCharts 內部 redraw 後 ref 仍是同 instance（保險）
		},
	},
	annotations: {
		yaxis: initialAverageLineAnnotations,
	},
	colors: [...props.chart_config.color],
	dataLabels: {
		// chart_config.showDataLabels 顯式覆寫；未設時 fallback 既有邏輯（categories 形式預設關，避免 stacked 多 series 數字疊在一起）
		enabled: typeof props.chart_config.showDataLabels === "boolean"
			? props.chart_config.showDataLabels
			: !props.chart_config.categories,
		// position=top 時 offsetY 推離條柱頂端外側 ~15px；搭配 grid.padding.top 讓最高條柱不頂卡片邊
		offsetY: props.chart_config.showDataLabels === true ? -15 : -20,
		style: {
			colors: ["var(--color-normal-text)"],
		},
	},
	grid: {
		show: false,
		// showDataLabels=true 時上方留 25px 給數字標籤；其他狀況不放 padding 屬性（避免 ApexCharts deep-merge undefined 邊界 case）
		...(props.chart_config.showDataLabels === true ? { padding: { top: 25 } } : {}),
	},
	legend: isLargeDataSet.value
		? {
			show: props.chart_config.categories ? true : false,
			horizontalAlign: "left",
			offsetX: 20,
			floating: true,
		  }
		: {
			show: props.chart_config.categories ? true : false,
		  },
	plotOptions: {
		bar: {
			borderRadius: 5,
			borderRadiusApplication: "end", // 只圓化條柱頂端，跟 BarChart 一致圓頭
			dataLabels: {
				hideOverflowingLabels: false,
				// showDataLabels=true 時把數字放到條柱「上方」（外部頂端）；
				// 未設或 false 走 ApexCharts 預設 center（條柱內部）
				position: props.chart_config.showDataLabels === true ? "top" : "center",
			},
		},
	},
	stroke: {
		colors: ["#282a2c"],
		show: true,
		width: 2,
	},
	tooltip: {
		// The class "chart-tooltip" could be edited in /assets/styles/chartStyles.css
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
						`${
							props.chart_config.categories
								? "-" + w.globals.seriesNames[seriesIndex]
								: ""
						}` +
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
		categories: props.chart_config.categories
			? props.chart_config.categories
			: [],
		labels: {
			offsetY: 2,
		},
		type: "category",
	},
});

// 切 dropdown 後 chart_config.average_line 被 mutation patch，watch 觸發即時呼叫 ApexCharts API 重畫
watch(averageLine, applyAverageAnnotation, { deep: true });

const selectedIndex = ref(null);

function handleDataSelection(_e, _chartContext, config) {
	if (!props.map_filter || !props.map_filter_on) {
		return;
	}
	if (
		`${config.dataPointIndex}-${config.seriesIndex}` !== selectedIndex.value
	) {
		// Supports filtering by xAxis + yAxis
		if (props.map_filter.mode === "byParam") {
			emits(
				"filterByParam",
				props.map_filter,
				props.map_config,
				config.w.globals.labels[config.dataPointIndex],
				config.w.globals.seriesNames[config.seriesIndex]
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

function increaseWidth() {
	widthValue.value += 50;
}

function decreaseWidth() {
	if (widthValue.value > 150) {
		widthValue.value -= 50;
	}
}

function resetWidth() {
	widthValue.value = initialWidth.value;
}
</script>

<template>
  <div
    v-if="activeChart === 'ColumnChart'"
    class="columnChart"
  >
    <div
      v-if="showAvgCorner"
      class="columnChart-avg-corner"
      :style="{ color: avgLineColor }"
    >
      {{ avgLineLabel }}
    </div>
    <div
      v-if="isLargeDataSet"
      class="columnChart-toolbar"
    >
      <p
        class="columnChart-toolbar-item"
        @click="increaseWidth"
      >
        <span>add</span>
      </p>
      <p
        class="columnChart-toolbar-item"
        @click="decreaseWidth"
      >
        <span>remove</span>
      </p>
      <p
        class="columnChart-toolbar-item reset"
        @click="resetWidth"
      >
        重置
      </p>
    </div>
    <VueApexCharts
      :key="chartWidth"
      type="bar"
      :width="chartWidth"
      height="250px"
      :options="chartOptions"
      :series="series"
      @data-point-selection="handleDataSelection"
    />
  </div>
</template>

<style lang="scss" scoped>
.columnChart {
	overflow: auto;
	position: relative;
	height: 100%;

	.vue-apexcharts {
		justify-content: unset !important;
	}

	&-avg-corner {
		position: absolute;
		top: 4px;
		right: 8px;
		font-size: var(--font-s);
		z-index: 2;
		pointer-events: none;
		white-space: nowrap;
	}

	&-toolbar {
		position: sticky;
		top: 0;
		left: 0;
		z-index: 1;
		background-color: var(--color-component-background);
		display: flex;
		justify-content: flex-end;
		align-items: center;
		gap: 4px;

		&-item {
			cursor: pointer;
			font-size: var(--font-s);
			display: flex;
			justify-content: center;
			align-items: center;

			span {
				text-align: center;
				font-family: var(--font-icon);
				font-size: var(--font-ms);
				padding: 2px;
			}

			&.reset {
				color: var(--color-highlight)
			}
		}
	}
}
</style>
