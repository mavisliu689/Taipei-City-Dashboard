<!-- 小碳寶對話面板 -->
<script setup>
import { computed, nextTick, ref, watch } from "vue";

import {
	calcDiningSavingG,
	calcLodgingSavingG,
	calcTransportSavingG,
	useEcoAssistantStore,
} from "../../store/ecoAssistantStore.js";

const TREE_KG_PER_YEAR = 21.77;

import CarbonBuddy from "./CarbonBuddy.vue";
import EcoMessageList from "./EcoMessageList.vue";

const store = useEcoAssistantStore();
const inputText = ref("");
const listRef = ref(null);
// Tier-2 任務面板狀態; null = 顯示 Tier-1 任務選單。
// 只在 store.currentRoute 存在時生效, 起始破冰階段不分層。
const taskTier = ref(null);
// 「整日綠生活」mini-form 狀態; 在外吃幾餐 (0-3) / 過夜幾晚 (0-1)
const lifestyleMeals = ref(2);
const lifestyleNights = ref(1);

const lifestylePreview = computed(() => {
	if (!store.currentRoute) return null;
	const tG = calcTransportSavingG(store.currentRoute, "car");
	const dG = calcDiningSavingG(lifestyleMeals.value);
	const lG = calcLodgingSavingG(lifestyleNights.value);
	const totalG = tG + dG + lG;
	return {
		kg: totalG / 1000,
		trees: totalG / 1000 / TREE_KG_PER_YEAR,
	};
});

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

// Tier-1: 4 顆主任務 chip (換路線 / 附近找什麼 / 看減碳 / 重新查詢)
const TIER1_TASKS = [
	{ key: "route", label: "換路線", icon: "alt_route" },
	{ key: "poi", label: "附近找什麼", icon: "place" },
	{ key: "carbon", label: "看減碳", icon: "eco" },
];

// Tier-2: 點到任務後展開的子選項
function tier2Suggestions(taskKey) {
	const origin = store.currentRoute?.start_name || "起點";
	switch (taskKey) {
		case "route":
			return [
				{ label: "→ 陽明山", action: "changeDest", target: "陽明山" },
				{ label: "→ 台北101", action: "changeDest", target: "台北101" },
				{ label: "→ 淡水", action: "changeDest", target: "淡水" },
				{ label: "→ 板橋", action: "changeDest", target: "板橋" },
			];
		case "poi":
			return [
				{ label: "公園", action: "send", text: `${origin}附近的公園` },
				{ label: "環保餐廳", action: "send", text: `${origin}附近的環保餐廳` },
				{ label: "回收站", action: "send", text: "沿途有什麼回收站" },
				{ label: "步道", action: "send", text: `${origin}附近的步道` },
				{ label: "YouBike", action: "send", text: `${origin}附近的 YouBike 站` },
			];
		case "carbon":
			return [
				{ label: "這條路省了多少碳", action: "send", text: "這條路省了多少碳" },
			];
		default:
			return [];
	}
}

const tier2Hint = computed(() => {
	switch (taskTier.value) {
		case "route": return "點目的地或直接輸入新的終點 (例：→ 信義區)";
		case "poi": return "選類別; 沒有想要的就直接輸入";
		case "carbon": return "選對照方式";
		default: return "";
	}
});

const tier2Label = computed(() => {
	switch (taskTier.value) {
		case "route": return "換路線";
		case "poi": return "附近找什麼";
		case "carbon": return "看減碳";
		default: return "";
	}
});

// 沒路線時的破冰範例 — 不分層, 直接平鋪
const welcomeChips = [
	{ label: "從台北市政府走到新北市政府", action: "send", text: "從台北市政府走到新北市政府" },
	{ label: "信義區的環保餐廳", action: "send", text: "信義區的環保餐廳" },
	{ label: "台北車站附近的公園", action: "send", text: "台北車站附近的公園" },
	{ label: "大安區附近的回收站", action: "send", text: "大安區附近的回收站" },
	{ label: "中山區附近的 YouBike 站", action: "send", text: "中山區附近的 YouBike 站" },
	{ label: "中山區附近的環保咖啡廳", action: "send", text: "中山區附近的環保咖啡廳" },
];

function isCarbonQuery(text) {
	return /省了?多少碳|減碳量|相當於.*棵樹|算.*碳/.test(text || "");
}

// 從文字抽 POI 類別關鍵字 → 對應 BE category。回 null 表非 POI 查詢。
function detectNearbyCategory(text) {
	if (!text) return null;
	if (/公園|綠地/.test(text)) return "park";
	if (/環保餐廳|餐廳|餐館|咖啡廳|咖啡店|咖啡館/.test(text)) return "restaurant";
	if (/環保旅館|旅館|飯店|民宿/.test(text)) return "hotel";
	if (/回收站|回收點|資源回收/.test(text)) return "recycle";
	if (/YouBike|youbike|Ubike|ubike|UBike|U-bike|微笑單車|共享單車|公共自行車|單車站|腳踏車站/.test(text)) return "ubike";
	return null;
}

// 「附近 / 沿途 + 類別」且我們已知起點座標 → 走 FE 直查路徑, 不要餵 LLM
async function maybeHandleNearbyChip(text) {
	if (!/附近|沿途/.test(text)) return false;
	const cat = detectNearbyCategory(text);
	if (!cat) return false;
	const haveCoord = store.manualOrigin || store.currentRoute?.start_coord;
	if (!haveCoord) return false;
	// 沿途查回收站時擴大半徑到 2km, 一般「附近」用 1km
	const radiusKm = /沿途/.test(text) ? 2 : 1;
	return await store.injectNearbyPOIReply(text, cat, { radiusKm });
}

async function sendSuggestion(s) {
	switch (s.action) {
		case "send":
			// 「省了多少碳」FE 端直接算秒回, 不打 LLM 卡住
			if (isCarbonQuery(s.text) && store.currentRoute) {
				store.injectCarbonReply(s.text);
				taskTier.value = null;
				return;
			}
			// 「附近的 X」+ 已有起點座標 → FE 直查, 避免 LLM 不認識 "自訂起點" 亂猜
			if (await maybeHandleNearbyChip(s.text)) {
				taskTier.value = null;
				return;
			}
			store.send(s.text);
			taskTier.value = null;
			break;
		case "totalCarbon":
			if (store.currentRoute) {
				store.injectTotalCarbonReply(s.text, {
					baselineKey: "car",
					meals: s.meals ?? lifestyleMeals.value,
					nights: s.nights ?? lifestyleNights.value,
				});
				taskTier.value = null;
			}
			break;
		case "changeDest": {
			const origin = store.currentRoute?.start_name || "目前位置";
			store.send(`從${origin}到${s.target}`);
			taskTier.value = null;
			break;
		}
		case "reset":
			store.reset();
			taskTier.value = null;
			break;
		case "openTier":
			taskTier.value = s.target;
			break;
		case "backTier":
			taskTier.value = null;
			break;
	}
}

async function handleSubmit() {
	if (!canSend.value) return;
	const text = inputText.value.trim();
	inputText.value = "";
	taskTier.value = null;
	if (isCarbonQuery(text) && store.currentRoute) {
		store.injectCarbonReply(text);
		return;
	}
	if (await maybeHandleNearbyChip(text)) return;
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

async function scrollToBottom() {
	await nextTick();
	// 第二個 tick 等 EcoMessageList / RouteCard 真正渲染完, 否則 scrollHeight
	// 還是舊值, 會停在中間
	await nextTick();
	if (listRef.value) {
		listRef.value.scrollTop = listRef.value.scrollHeight;
	}
}

// 新訊息 / 串流字數成長都觸發捲底, 確保使用者跟得上
watch(() => store.messages.length, scrollToBottom);

// 打開面板時先把畫面捲到最底, 不要停在上次離開的位置
watch(() => store.open, (open) => {
	if (open) scrollToBottom();
}, { immediate: true });

// 路線消失 (reset 或新對話) 時, Tier-2 也要收起來避免懸空
watch(() => store.currentRoute, (route) => {
	if (!route) taskTier.value = null;
});
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

		<!-- 對話引導建議 -->
		<div
			v-if="!store.isStreaming"
			class="eco-panel__suggestions"
		>
			<!-- 沒路線 — 平鋪破冰範例 -->
			<template v-if="!store.currentRoute">
				<button
					v-for="s in welcomeChips"
					:key="s.label"
					class="eco-panel__chip"
					@click="sendSuggestion(s)"
				>
					{{ s.label }}
				</button>
			</template>
			<!-- 有路線 + Tier-1 — 3 顆主任務 + 重新查詢 -->
			<template v-else-if="taskTier === null">
				<button
					v-for="t in TIER1_TASKS"
					:key="t.key"
					class="eco-panel__chip eco-panel__chip--task"
					@click="sendSuggestion({ action: 'openTier', target: t.key })"
				>
					<span class="material-icons-round">{{ t.icon }}</span>
					{{ t.label }}
				</button>
				<button
					class="eco-panel__chip eco-panel__chip--reset"
					@click="sendSuggestion({ action: 'reset' })"
				>
					重新查詢
				</button>
			</template>
			<!-- 有路線 + Tier-2 — 子選項 + 返回 -->
			<template v-else>
				<div class="eco-panel__tier2-bar">
					<button
						class="eco-panel__back"
						@click="sendSuggestion({ action: 'backTier' })"
					>
						<span class="material-icons-round">arrow_back</span>
						{{ tier2Label }}
					</button>
					<span class="eco-panel__tier2-hint">{{ tier2Hint }}</span>
				</div>
				<button
					v-for="s in tier2Suggestions(taskTier)"
					:key="s.label"
					class="eco-panel__chip"
					@click="sendSuggestion(s)"
				>
					{{ s.label }}
				</button>
				<!-- 看減碳 Tier-2: 整日綠生活 mini-form -->
				<div
					v-if="taskTier === 'carbon'"
					class="eco-lifestyle"
				>
					<div class="eco-lifestyle__title">
						<span class="material-icons-round">eco</span>
						整日綠生活減碳
					</div>
					<div class="eco-lifestyle__row">
						<span class="eco-lifestyle__label">在外吃</span>
						<div class="eco-lifestyle__seg">
							<button
								v-for="n in 4"
								:key="n - 1"
								:class="{ 'is-on': lifestyleMeals === n - 1 }"
								@click="lifestyleMeals = n - 1"
							>
								{{ n - 1 }}
							</button>
						</div>
						<span class="eco-lifestyle__suffix">餐</span>
					</div>
					<div class="eco-lifestyle__row">
						<span class="eco-lifestyle__label">過夜</span>
						<div class="eco-lifestyle__seg">
							<button
								:class="{ 'is-on': lifestyleNights === 0 }"
								@click="lifestyleNights = 0"
							>否</button>
							<button
								:class="{ 'is-on': lifestyleNights === 1 }"
								@click="lifestyleNights = 1"
							>是</button>
						</div>
					</div>
					<div
						v-if="lifestylePreview"
						class="eco-lifestyle__preview"
					>
						預估省 <strong>{{ lifestylePreview.kg.toFixed(2) }}</strong> kg CO₂e
						<span class="eco-lifestyle__sub">≈ {{ lifestylePreview.trees.toFixed(2) }} 棵樹一年固碳</span>
					</div>
					<button
						class="eco-lifestyle__go"
						@click="sendSuggestion({
							action: 'totalCarbon',
							text: `估一日總減碳：交通 + ${lifestyleMeals} 餐 + ${lifestyleNights} 晚`,
							meals: lifestyleMeals,
							nights: lifestyleNights,
						})"
					>
						顯示在對話
					</button>
				</div>
			</template>
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
		display: inline-flex;
		align-items: center;
		gap: 4px;

		&:hover {
			background: rgba(102, 187, 106, 0.25);
		}

		.material-icons-round {
			font-size: 13px !important;
		}

		// 任務級 chip — 視覺加重一階, 表達主層級
		&--task {
			background: rgba(0, 184, 169, 0.18);
			border-color: rgba(0, 184, 169, 0.55);
			color: #a7ece5;
			font-size: 12px;
			font-weight: 600;
			padding: 6px 12px;

			&:hover {
				background: rgba(0, 184, 169, 0.32);
			}
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

	&__tier2-bar {
		flex: 0 0 100%;
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 4px;
	}

	&__back {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.15);
		color: #ddd;
		border-radius: 12px;
		padding: 3px 9px;
		font-size: 11px;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.15s;

		.material-icons-round {
			font-size: 14px !important;
		}

		&:hover {
			background: rgba(255, 255, 255, 0.12);
		}
	}

	&__tier2-hint {
		font-size: 10px;
		color: #888;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
}

.eco-lifestyle {
	flex: 0 0 100%;
	margin-top: 6px;
	padding: 10px 12px;
	background: rgba(93, 192, 124, 0.07);
	border: 1px solid rgba(93, 192, 124, 0.25);
	border-radius: 12px;
	display: flex;
	flex-direction: column;
	gap: 7px;

	&__title {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12px;
		font-weight: 700;
		color: #c0e8c8;

		.material-icons-round {
			font-size: 14px !important;
			color: #5dc07c;
		}
	}

	&__row {
		display: flex;
		align-items: center;
		gap: 7px;
	}

	&__label {
		font-size: 11px;
		color: #bbb;
		min-width: 38px;
	}

	&__suffix {
		font-size: 11px;
		color: #888;
	}

	&__seg {
		display: inline-flex;
		background: rgba(0, 0, 0, 0.25);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 999px;
		padding: 2px;

		button {
			background: transparent;
			border: none;
			color: #aaa;
			cursor: pointer;
			padding: 3px 10px;
			font-size: 11px;
			font-weight: 600;
			border-radius: 999px;
			transition: all 0.15s;
			min-width: 26px;
			font-variant-numeric: tabular-nums;

			&:hover { color: #fff; }

			&.is-on {
				background: linear-gradient(135deg, #5dc07c, #3fa85f);
				color: #0d1411;
			}
		}
	}

	&__preview {
		font-size: 12px;
		color: #ddd;
		padding: 6px 8px;
		background: rgba(0, 0, 0, 0.2);
		border-radius: 8px;
		line-height: 1.5;

		strong {
			color: #5dc07c;
			font-size: 14px;
			font-weight: 800;
			font-variant-numeric: tabular-nums;
		}
	}

	&__sub {
		display: block;
		font-size: 10px;
		color: #888;
		margin-top: 1px;
	}

	&__go {
		align-self: stretch;
		background: linear-gradient(135deg, #5dc07c, #3fa85f);
		color: #0d1411;
		border: none;
		border-radius: 10px;
		padding: 6px 10px;
		font-size: 12px;
		font-weight: 700;
		cursor: pointer;
		transition: filter 0.15s;

		&:hover { filter: brightness(1.08); }
	}
}

.eco-panel {
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
