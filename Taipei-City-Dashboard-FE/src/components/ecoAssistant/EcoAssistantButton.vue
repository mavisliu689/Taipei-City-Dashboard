<!-- Floating CarbonBuddy launcher that toggles the eco-route assistant panel -->
<script setup>
import { computed } from "vue";

import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

import CarbonBuddy from "./CarbonBuddy.vue";

const store = useEcoAssistantStore();

// Buddy expression mirrors the active assistant state — even when collapsed.
const buddyState = computed(() => {
	if (store.isStreaming) return "thinking";
	if (store.currentRoute) return "found";
	return "idle";
});

// 只要不是 idle 就把 decor 開起來 — 包含面板打開中, 讓使用者在對話過程
// 也能從右下角小碳寶看到 thinking / found 等狀態動畫。
const showDecor = computed(() => buddyState.value !== "idle");
</script>

<template>
	<button
		class="eco-fab"
		:class="[`eco-fab--state-${buddyState}`, { 'eco-fab--active': store.open }]"
		aria-label="小碳寶"
		@click="store.togglePanel"
	>
		<CarbonBuddy
			class="eco-fab__buddy"
			:state="buddyState"
			:size="56"
			:decor="showDecor"
		/>
		<span class="eco-fab__pulse" />
	</button>
</template>

<style scoped lang="scss">
.eco-fab {
	position: relative;
	width: 56px;
	height: 56px;
	padding: 0;
	background: transparent;
	border: none;
	cursor: pointer;
	display: flex;
	align-items: center;
	justify-content: center;
	transition: transform 0.2s ease;
	overflow: visible;

	&__buddy {
		z-index: 1;
	}

	&__pulse {
		position: absolute;
		inset: -4px;
		border-radius: 50%;
		// pulse 顏色會隨 state 變: idle=青綠 / thinking=紫 / searching=藍 / found=金 / offline=灰
		border: 2px solid var(--cb-pulse-color, rgba(129, 216, 208, 0.6));
		animation: ecoPulse 2.4s ease-out infinite;
		transition: border-color 0.35s ease;
	}

	&:hover {
		transform: scale(1.08);
	}

	&--active {
		transform: scale(1.05);
	}

	// 各 state 對應的 pulse 顏色 — 與 CarbonBuddy ring 的色系保持一致
	&--state-idle    { --cb-pulse-color: rgba(129, 216, 208, 0.6); }
	&--state-typing  { --cb-pulse-color: rgba(120, 210, 145, 0.65); }
	&--state-thinking {
		--cb-pulse-color: rgba(160, 130, 240, 0.7);
		.eco-fab__pulse { animation-duration: 1.6s; }
	}
	&--state-searching {
		--cb-pulse-color: rgba(80, 150, 230, 0.7);
		.eco-fab__pulse { animation-duration: 1.6s; }
	}
	&--state-found {
		--cb-pulse-color: rgba(255, 195, 70, 0.85);
		.eco-fab__pulse { animation-duration: 1.4s; }
	}
	&--state-offline { --cb-pulse-color: rgba(160, 168, 180, 0.5); }
}

@keyframes ecoPulse {
	0% {
		transform: scale(1);
		opacity: 0.7;
	}
	100% {
		transform: scale(1.6);
		opacity: 0;
	}
}
</style>