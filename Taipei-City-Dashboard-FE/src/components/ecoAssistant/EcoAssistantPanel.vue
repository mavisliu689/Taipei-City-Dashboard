<!-- 小碳寶對話面板 -->
<script setup>
import { computed, nextTick, ref, watch } from "vue";

import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

import CarbonBuddy from "./CarbonBuddy.vue";
import EcoMessageList from "./EcoMessageList.vue";

const store = useEcoAssistantStore();
const inputText = ref("");
const listRef = ref(null);

const canSend = computed(() => inputText.value.trim() && !store.isStreaming);

// Header buddy reflects current assistant phase; status text mirrors it.
const headerState = computed(() => {
	if (store.isStreaming) {
		const last = store.messages[store.messages.length - 1];
		if (last?.routeReady) return "found";
		return "thinking";
	}
	if (store.currentRoute) return "found";
	return "idle";
});
const headerStatusText = computed(() => {
	switch (headerState.value) {
		case "thinking": return "思考中…";
		case "found": return "為你找到路線了";
		default: return "線上中";
	}
});

// 持久顯示的對話建議 (任何時候都可點)
// 三大功能: A→B 路線 / 附近 POI 搜尋 / 這條路省多少碳
const suggestions = computed(() => {
	if (store.currentRoute) {
		// 已有路線時, 建議聚焦於延伸動作 (使用當前 origin 上下文)
		const origin = store.currentRoute.start_name || "起點";
		return [
			{ label: "換目的地→陽明山", action: "changeDest", target: "陽明山" },
			{ label: "換目的地→台北101", action: "changeDest", target: "台北101" },
			{ label: "沿途有什麼回收站", action: "send", text: "沿途有什麼回收站" },
			{ label: "附近的公園", action: "send", text: `${origin}附近的公園` },
			{ label: "這條路省了多少碳", action: "send", text: "這條路省了多少碳" },
			{ label: "重新查詢", action: "reset" },
		];
	}
	return [
		{ label: "從台北市政府走到新北市政府", action: "send", text: "從台北市政府走到新北市政府" },
		{ label: "目前位置到陽明山", action: "send", text: "目前位置到陽明山" },
		{ label: "信義區的環保餐廳", action: "send", text: "信義區的環保餐廳" },
		{ label: "台北車站附近的公園", action: "send", text: "台北車站附近的公園" },
		{ label: "大安區附近的回收站", action: "send", text: "大安區附近的回收站" },
		{ label: "中山區附近的環保咖啡廳", action: "send", text: "中山區附近的環保咖啡廳" },
	];
});

function isCarbonQuery(text) {
	return /省了?多少碳|減碳量|相當於.*棵樹|算.*碳/.test(text || "");
}

function sendSuggestion(s) {
	switch (s.action) {
		case "send":
			// 「省了多少碳」FE 端直接算秒回, 不打 LLM 卡住
			if (isCarbonQuery(s.text) && store.currentRoute) {
				store.injectCarbonReply(s.text);
				return;
			}
			store.send(s.text);
			break;
		case "changeDest": {
			const origin = store.currentRoute?.start_name || "目前位置";
			store.send(`從${origin}到${s.target}`);
			break;
		}
		case "reset":
			store.reset();
			break;
	}
}

async function handleSubmit() {
	if (!canSend.value) return;
	const text = inputText.value.trim();
	inputText.value = "";
	if (isCarbonQuery(text) && store.currentRoute) {
		store.injectCarbonReply(text);
		return;
	}
	await store.send(text);
}

function handleKeydown(e) {
	// 中文 IME 組字中的 Enter 會把未完成字串提早送出 → 跳過
	// (e.isComposing 標準 / e.keyCode 229 為 Safari + 舊瀏覽器後備)
	if (e.isComposing || e.keyCode === 229) return;
	if (e.key === "Enter" && !e.shiftKey) {
		e.preventDefault();
		handleSubmit();
	}
}

watch(
	() => store.messages.length,
	async () => {
		await nextTick();
		if (listRef.value) {
			listRef.value.scrollTop = listRef.value.scrollHeight;
		}
	}
);
</script>

<template>
	<div
		v-if="store.open"
		class="eco-panel"
		role="dialog"
		aria-label="小碳寶對話"
	>
		<header class="eco-panel__header">
			<div class="eco-panel__title">
				<div class="eco-panel__avatar">
					<CarbonBuddy
						:state="headerState"
						:size="40"
						:decor="false"
					/>
					<span class="eco-panel__avatar-dot" />
				</div>
				<div class="eco-panel__title-text">
					<span class="eco-panel__name">小碳寶</span>
					<span class="eco-panel__status">{{ headerStatusText }}</span>
				</div>
			</div>
			<div class="eco-panel__actions">
				<button
					class="eco-panel__icon-btn"
					title="清除對話"
					@click="store.reset"
				>
					<span class="material-icons-round">refresh</span>
				</button>
				<button
					class="eco-panel__icon-btn"
					title="關閉"
					@click="store.closePanel"
				>
					<span class="material-icons-round">close</span>
				</button>
			</div>
		</header>

		<section
			ref="listRef"
			class="eco-panel__body"
		>
			<div
				v-if="store.messages.length === 0"
				class="eco-panel__welcome"
			>
				<div class="eco-panel__welcome-stage">
					<CarbonBuddy
						state="idle"
						:size="170"
						:ring="false"
						decor
					/>
				</div>
				<p class="eco-panel__welcome-greet">嗨，我是雙北小碳寶！</p>
				<p class="eco-panel__welcome-sub">試試問我：</p>
			</div>
			<EcoMessageList :messages="store.messages" />
			<div
				v-if="store.errorMessage"
				class="eco-panel__error"
			>
				⚠️ {{ store.errorMessage }}
			</div>
		</section>

		<!-- 對話引導建議 — 任何時候都顯示 (不只破冰) -->
		<div
			v-if="!store.isStreaming"
			class="eco-panel__suggestions"
		>
			<button
				v-for="s in suggestions"
				:key="s.label"
				class="eco-panel__chip"
				:class="{ 'eco-panel__chip--reset': s.action === 'reset' }"
				@click="sendSuggestion(s)"
			>
				{{ s.label }}
			</button>
		</div>

		<footer class="eco-panel__footer">
			<textarea
				v-model="inputText"
				class="eco-panel__input"
				rows="2"
				placeholder="輸入你想去的地方..."
				:disabled="store.isStreaming"
				@keydown="handleKeydown"
			/>
			<button
				class="eco-panel__send"
				:disabled="!canSend"
				@click="handleSubmit"
			>
				<span class="material-icons-round">send</span>
			</button>
		</footer>
	</div>
</template>

<style scoped lang="scss">
.eco-panel {
	position: fixed;
	// 預留下方兩個 launcher (小碳寶 56px + 舊 chatbot 70px + 間距) 的空間
	bottom: 11rem;
	right: 1.5rem;
	width: 380px;
	height: 560px;
	background: #1e1e1e;
	border-radius: 16px;
	box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
	display: flex;
	flex-direction: column;
	overflow: hidden;
	z-index: 11;
	border: 1px solid rgba(102, 187, 106, 0.3);

	&__header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 16px;
		background: linear-gradient(135deg, #81d8d0, #00b8a9);
		color: #fff;
	}

	&__title {
		display: flex;
		align-items: center;
		gap: 10px;
		font-weight: 600;
	}

	&__avatar {
		position: relative;
		width: 40px;
		height: 40px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex: none;
	}

	&__avatar-dot {
		position: absolute;
		right: 0;
		bottom: 0;
		width: 11px;
		height: 11px;
		border-radius: 50%;
		background: #4ade80;
		border: 2px solid #00b8a9;
	}

	&__title-text {
		display: flex;
		flex-direction: column;
		line-height: 1.15;
	}

	&__name {
		font-size: 15px;
		font-weight: 700;
	}

	&__status {
		font-size: 11px;
		opacity: 0.95;
		margin-top: 2px;
		display: flex;
		align-items: center;
		gap: 5px;

		&::before {
			content: "";
			width: 6px;
			height: 6px;
			border-radius: 50%;
			background: #4ade80;
		}
	}

	&__actions {
		display: flex;
		gap: 4px;
	}

	&__icon-btn {
		background: transparent;
		border: none;
		color: #fff;
		cursor: pointer;
		padding: 4px;
		display: flex;
		align-items: center;
		opacity: 0.85;

		&:hover {
			opacity: 1;
		}

		.material-icons-round {
			font-size: 20px;
		}
	}

	&__body {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
		color: #e0e0e0;
		font-size: 14px;
		line-height: 1.55;
	}

	&__welcome {
		color: #aaa;
		font-size: 13px;
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		padding: 8px 0 12px;
	}

	&__welcome-stage {
		width: 200px;
		height: 200px;
		border-radius: 50%;
		// 強調的青綠 disc 背景, 讓綠身小碳寶有對比度
		background: radial-gradient(circle at 30% 28%, #a6e8e0 0%, #81d8d0 60%, #5ec4bb 100%);
		box-shadow:
			0 8px 24px rgba(0, 119, 182, 0.25),
			inset 0 -6px 16px rgba(0, 0, 0, 0.08);
		display: flex;
		align-items: center;
		justify-content: center;
		margin-bottom: 14px;
		position: relative;
		overflow: hidden;

		&::after {
			content: "";
			position: absolute;
			inset: 0;
			border-radius: inherit;
			background: radial-gradient(
				circle at 75% 90%,
				rgba(255, 255, 255, 0.18),
				transparent 50%
			);
			pointer-events: none;
		}
	}

	&__welcome-greet {
		color: #fff;
		font-size: 16px;
		font-weight: 700;
		margin: 0 0 4px;
	}

	&__welcome-sub {
		color: #aaa;
		font-size: 12px;
		margin: 0;
	}

	&__suggestions {
		padding: 8px 12px 0;
		display: flex;
		flex-wrap: wrap;
		gap: 5px;
		border-top: 1px solid rgba(255, 255, 255, 0.06);
	}

	&__chip {
		background: rgba(102, 187, 106, 0.1);
		border: 1px solid rgba(102, 187, 106, 0.35);
		color: #b9f6ca;
		border-radius: 14px;
		padding: 4px 10px;
		cursor: pointer;
		font-size: 11px;
		transition: background 0.15s;

		&:hover {
			background: rgba(102, 187, 106, 0.25);
		}

		&--reset {
			background: rgba(255, 100, 100, 0.08);
			border-color: rgba(255, 100, 100, 0.4);
			color: #ef9a9a;

			&:hover {
				background: rgba(255, 100, 100, 0.18);
			}
		}
	}

	&__error {
		margin-top: 12px;
		padding: 10px;
		background: rgba(255, 87, 87, 0.1);
		border-left: 3px solid #ef5350;
		border-radius: 4px;
		color: #ffcdd2;
		font-size: 13px;
	}

	&__footer {
		padding: 12px;
		border-top: 1px solid rgba(255, 255, 255, 0.1);
		display: flex;
		gap: 8px;
		align-items: flex-end;
	}

	&__input {
		flex: 1;
		background: #2a2a2a;
		border: 1px solid #444;
		border-radius: 12px;
		color: #fff;
		padding: 10px 12px;
		resize: none;
		font-family: inherit;
		font-size: 14px;

		&:focus {
			outline: none;
			border-color: #00b8a9;
		}

		&:disabled {
			opacity: 0.6;
		}
	}

	&__send {
		width: 40px;
		height: 40px;
		border-radius: 50%;
		background: linear-gradient(135deg, #81d8d0, #00b8a9);
		color: #fff;
		border: none;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;

		&:disabled {
			opacity: 0.4;
			cursor: not-allowed;
		}

		&:not(:disabled):hover {
			background: linear-gradient(135deg, #5ec4bb, #00a89a);
		}
	}
}

@media (max-width: 600px) {
	.eco-panel {
		width: calc(100vw - 2rem);
		right: 1rem;
		// 手機版舊 chatbot 隱藏, 只需避開小碳寶 launcher
		bottom: 8rem;
	}
}
</style>
