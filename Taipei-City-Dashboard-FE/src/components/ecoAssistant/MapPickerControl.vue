<!--
  MapPickerControl — 浮動 widget 讓使用者在地圖上點選/拖曳起終點。
  與 EcoMapOverlay 配合：
   - 此元件管 UI 與 pickMode 狀態
   - EcoMapOverlay 監聽 pickMode 攔截 map click + 渲染拖曳 marker
-->
<script setup>
import { computed, watch } from "vue";
import { useRoute } from "vue-router";

import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

const ecoStore = useEcoAssistantStore();
const route = useRoute();

const onMapView = computed(() => route.path === "/mapview");

const originLabel = computed(() => {
	if (!ecoStore.manualOrigin) return "點地圖選起點";
	const { lat, lng } = ecoStore.manualOrigin;
	return `${lat.toFixed(4)}, ${lng.toFixed(4)}`;
});
const destLabel = computed(() => {
	if (!ecoStore.manualDest) return "點地圖選終點";
	const { lat, lng } = ecoStore.manualDest;
	return `${lat.toFixed(4)}, ${lng.toFixed(4)}`;
});

function pickOrigin() {
	ecoStore.setPickMode(ecoStore.pickMode === "origin" ? "idle" : "origin");
}
function pickDest() {
	ecoStore.setPickMode(ecoStore.pickMode === "destination" ? "idle" : "destination");
}
function clear() {
	ecoStore.clearManualPoints();
}

// 切換頁面時自動取消選點
watch(
	() => route.path,
	() => {
		if (ecoStore.pickMode !== "idle") ecoStore.setPickMode("idle");
	}
);
</script>

<template>
	<div
		v-if="onMapView"
		class="map-picker"
	>
		<header class="map-picker__header">
			<span class="material-icons-round">touch_app</span>
			<span>地圖點選起終點</span>
		</header>

		<button
			class="map-picker__btn map-picker__btn--origin"
			:class="{ active: ecoStore.pickMode === 'origin', filled: !!ecoStore.manualOrigin }"
			@click="pickOrigin"
		>
			<span class="material-icons-round">trip_origin</span>
			<span class="map-picker__btn-text">{{ originLabel }}</span>
			<span
				v-if="ecoStore.pickMode === 'origin'"
				class="map-picker__hint"
			>請點地圖</span>
		</button>

		<button
			class="map-picker__btn map-picker__btn--dest"
			:class="{ active: ecoStore.pickMode === 'destination', filled: !!ecoStore.manualDest }"
			@click="pickDest"
		>
			<span class="material-icons-round">place</span>
			<span class="map-picker__btn-text">{{ destLabel }}</span>
			<span
				v-if="ecoStore.pickMode === 'destination'"
				class="map-picker__hint"
			>請點地圖</span>
		</button>

		<button
			v-if="ecoStore.manualOrigin || ecoStore.manualDest"
			class="map-picker__clear"
			@click="clear"
		>
			<span class="material-icons-round">delete</span> 清除起終點
		</button>

		<p
			v-if="ecoStore.manualOrigin && ecoStore.manualDest"
			class="map-picker__tip"
		>
			✓ 已自動規劃路線；可拖曳 marker 微調
		</p>
	</div>
</template>

<style scoped lang="scss">
.map-picker {
	position: fixed;
	top: 16px;
	right: 80px;
	z-index: 30;
	background: rgba(20, 20, 20, 0.92);
	backdrop-filter: blur(8px);
	border: 1px solid rgba(102, 187, 106, 0.4);
	border-radius: 10px;
	padding: 10px 12px;
	min-width: 220px;
	color: #e0e0e0;
	box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
	font-size: 12px;
	user-select: none;
	display: flex;
	flex-direction: column;
	gap: 6px;

	&__header {
		display: flex;
		align-items: center;
		gap: 6px;
		font-weight: 600;
		color: #b9f6ca;
		font-size: 12px;
		margin-bottom: 4px;

		.material-icons-round {
			font-size: 16px;
		}
	}

	&__btn {
		display: flex;
		align-items: center;
		gap: 8px;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 8px;
		padding: 8px 10px;
		color: #ccc;
		cursor: pointer;
		font-size: 12px;
		font-family: inherit;
		transition: all 0.15s;
		text-align: left;

		.material-icons-round {
			font-size: 18px;
		}

		&:hover {
			background: rgba(255, 255, 255, 0.08);
		}

		&--origin .material-icons-round {
			color: #4fc3f7;
		}
		&--dest .material-icons-round {
			color: #ff8a65;
		}

		&.active {
			background: rgba(102, 187, 106, 0.18);
			border-color: rgba(102, 187, 106, 0.6);
			color: #fff;
		}

		&.filled {
			color: #fff;
		}
	}

	&__btn-text {
		flex: 1;
	}

	&__hint {
		font-size: 10px;
		color: #66bb6a;
		font-weight: 600;
	}

	&__clear {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 4px;
		background: transparent;
		border: 1px dashed rgba(255, 100, 100, 0.4);
		border-radius: 6px;
		padding: 5px;
		color: #ef9a9a;
		cursor: pointer;
		font-size: 11px;

		.material-icons-round {
			font-size: 14px;
		}

		&:hover {
			background: rgba(255, 100, 100, 0.1);
		}
	}

	&__tip {
		margin: 4px 0 0;
		font-size: 11px;
		color: #b9f6ca;
		text-align: center;
	}
}
</style>
