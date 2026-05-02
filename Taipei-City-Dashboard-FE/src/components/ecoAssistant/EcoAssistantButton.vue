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

// Show full-decor expression on the launcher only when the panel is closed
// (so users notice the AI is busy / has news), and only for non-idle states.
const showDecor = computed(() => !store.open && buddyState.value !== "idle");
</script>

<template>
	<button
		class="eco-fab"
		:class="{ 'eco-fab--active': store.open }"
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
		border: 2px solid rgba(129, 216, 208, 0.6);
		animation: ecoPulse 2.4s ease-out infinite;
	}

	&:hover {
		transform: scale(1.08);
		box-shadow: 0 6px 22px rgba(0, 184, 169, 0.55);
	}

	&--active {
		box-shadow: 0 6px 22px rgba(0, 184, 169, 0.65);
		transform: scale(1.05);
	}
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