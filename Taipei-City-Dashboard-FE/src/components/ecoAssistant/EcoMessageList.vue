<!-- 對話訊息串列 -->
<script setup>
import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

import CarbonBuddy from "./CarbonBuddy.vue";
import EcoRouteCard from "./EcoRouteCard.vue";

const props = defineProps({
	messages: {
		type: Array,
		required: true,
	},
});

const store = useEcoAssistantStore();

// State for the buddy avatar next to each assistant bubble.
function buddyStateFor(m, i) {
	const isLast = i === props.messages.length - 1;
	if (m.role !== "assistant") return "idle";
	if (isLast && store.isStreaming) {
		if (m.routeReady) return "found";
		if (m.content) return "typing";
		return "thinking";
	}
	if (m.routeData) return "found";
	return "idle";
}
</script>

<template>
	<ul class="eco-msg-list">
		<template
			v-for="(m, i) in messages"
			:key="i"
		>
			<li :class="['eco-msg', `eco-msg--${m.role}`]">
				<CarbonBuddy
					v-if="m.role === 'assistant'"
					class="eco-msg__avatar"
					:state="buddyStateFor(m, i)"
					:size="34"
					:decor="false"
				/>
				<div class="eco-msg__bubble">
					<span v-if="m.content">{{ m.content }}</span>
					<span
						v-else-if="m.role === 'assistant' && i === messages.length - 1 && store.isStreaming && m.routeReady"
						class="eco-msg__route-ready"
					>
						✓ 路線已產生，AI 正在撰寫說明…
						<span class="eco-msg__typing">
							<span /><span /><span />
						</span>
					</span>
					<span
						v-else-if="m.role === 'assistant' && i === messages.length - 1 && store.isStreaming"
						class="eco-msg__typing"
					>
						<span /><span /><span />
					</span>
					<span
						v-else-if="m.role === 'assistant'"
						class="eco-msg__empty"
					>（AI 沒有回應，請再試一次）</span>
				</div>
			</li>
			<!-- 路線資料 inline 跟在那則 assistant 訊息後面, 隨對話往上捲 -->
			<!-- 等 AI 文字開始/完成後才顯示 routecard, 避免 card 比文字早出現 -->
			<li
				v-if="m.role === 'assistant' && m.routeData && (m.content || !(i === messages.length - 1 && store.isStreaming))"
				class="eco-msg__route-attachment"
			>
				<EcoRouteCard :result="m.routeData" />
			</li>
		</template>
	</ul>
</template>

<style scoped lang="scss">
.eco-msg-list {
	list-style: none;
	padding: 0;
	margin: 0;
	display: flex;
	flex-direction: column;
	gap: 10px;
}

.eco-msg__route-attachment {
	margin-top: -4px;
	margin-left: 42px; // align under bubble (avatar 34 + 8 gap)
}

.eco-msg {
	display: flex;
	align-items: flex-end;
	gap: 8px;

	&__avatar {
		margin-bottom: 2px;
	}

	&__bubble {
		max-width: 80%;
		padding: 9px 12px;
		border-radius: 14px;
		font-size: 14px;
		white-space: pre-wrap;
		word-break: break-word;
	}

	&--user {
		justify-content: flex-end;

		.eco-msg__bubble {
			background: linear-gradient(135deg, #00b8a9, #009b8e);
			color: #fff;
			border-bottom-right-radius: 4px;
		}
	}

	&--assistant {
		justify-content: flex-start;

		.eco-msg__bubble {
			background: #2a2a2a;
			color: #e0e0e0;
			border: 1px solid #3a3a3a;
			border-bottom-left-radius: 4px;
		}
	}

	&__empty {
		color: #888;
		font-style: italic;
		font-size: 12px;
	}

	&__route-ready {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		color: #b9f6ca;
		font-size: 13px;
	}

	&__typing {
		display: inline-flex;
		gap: 3px;

		span {
			width: 6px;
			height: 6px;
			background: #00b8a9;
			border-radius: 50%;
			animation: ecoTypingDot 1.4s infinite ease-in-out;

			&:nth-child(2) {
				animation-delay: 0.2s;
			}

			&:nth-child(3) {
				animation-delay: 0.4s;
			}
		}
	}
}

@keyframes ecoTypingDot {
	0%, 80%, 100% {
		transform: scale(0.5);
		opacity: 0.4;
	}
	40% {
		transform: scale(1);
		opacity: 1;
	}
}
</style>