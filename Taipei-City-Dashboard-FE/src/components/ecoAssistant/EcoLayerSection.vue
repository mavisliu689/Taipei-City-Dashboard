<!--
  EcoLayerSection — 把 AI 對話產生的路線/景點以「圖層 toggle」形式
  插入 MapView 的左側 layer 列表，視覺與 DashboardComponent toggle 一致。

  選項 B 的整合方式：AI 對話結果 = 一筆可開關的圖層。
-->
<script setup>
import { computed } from "vue";

import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

const store = useEcoAssistantStore();

const route = computed(() => store.currentRoute);

const ROUTE_META = {
	shortest: { color: "#ffb74d", icon: "directions_walk" },
	balanced: { color: "#4fc3f7", icon: "balance" },
	greenest: { color: "#66bb6a", icon: "park" },
};

const search = computed(() => store.currentSearchPOIs);

const CATEGORY_LABELS = {
	park: "公園",
	restaurant: "環保餐廳",
	hotel: "環保旅館",
	trail: "登山步道",
	recycle: "回收站",
};

const items = computed(() => {
	const out = [];
	if (route.value) {
		for (const r of route.value.routes) {
			out.push({
				key: r.route_id,
				label: `綠能助手・${r.label}`,
				meta: `${(r.distance_m / 1000).toFixed(1)} km · ${r.estimated_minutes} 分 · 綠點 ${r.green_points_passed.length}`,
				color: ROUTE_META[r.route_id]?.color || "#888",
				icon: ROUTE_META[r.route_id]?.icon || "route",
			});
		}
		const seen = new Set();
		for (const r of route.value.routes) {
			for (const p of r.green_points_passed) seen.add(`${p.lat}|${p.lng}|${p.name}`);
		}
		out.push({
			key: "pois",
			label: "綠能助手・沿途景點",
			meta: `${seen.size} 個景點 marker`,
			color: "#66bb6a",
			icon: "place",
		});
	}
	if (search.value) {
		const cats = (search.value.categories || []).map((c) => CATEGORY_LABELS[c] || c).join("・");
		out.push({
			key: "search",
			label: `綠能助手・搜尋結果 (${cats || "POI"})`,
			meta: `${search.value.items?.length || 0} 個結果，半徑 ${search.value.radius} km`,
			color: "#fdd835",
			icon: "search",
		});
	}
	return out;
});

function toggle(key) {
	store.toggleLayer(key);
}
</script>

<template>
	<div
		v-if="items.length"
		class="eco-layer-section"
	>
		<div class="eco-layer-section__title">
			<span class="material-icons-round">eco</span>
			<span>綠能助手結果</span>
			<span class="eco-layer-section__od">
				{{ route.start_name }} → {{ route.end_name }}
			</span>
		</div>
		<div
			v-for="item in items"
			:key="item.key"
			class="eco-layer-row"
			@click="toggle(item.key)"
		>
			<span
				class="eco-layer-row__bar"
				:style="{ background: item.color }"
			/>
			<div class="eco-layer-row__icon-wrap">
				<span
					class="material-icons-round"
					:style="{ color: item.color }"
				>{{ item.icon }}</span>
			</div>
			<div class="eco-layer-row__text">
				<div class="eco-layer-row__label">
					{{ item.label }}
				</div>
				<div class="eco-layer-row__meta">
					{{ item.meta }}
				</div>
			</div>
			<label
				class="eco-layer-row__switch"
				:class="{ 'is-on': store.layerVisibility[item.key] }"
				@click.stop="toggle(item.key)"
			>
				<span class="eco-layer-row__knob" />
			</label>
		</div>
	</div>
</template>

<style scoped lang="scss">
.eco-layer-section {
	display: flex;
	flex-direction: column;
	gap: 6px;
	padding: 12px;
	background: rgba(46, 125, 50, 0.06);
	border: 1px solid rgba(102, 187, 106, 0.35);
	border-radius: 8px;
	margin-bottom: 8px;

	&__title {
		display: flex;
		align-items: center;
		gap: 6px;
		color: #b9f6ca;
		font-weight: 600;
		font-size: 13px;
		margin-bottom: 4px;

		.material-icons-round {
			font-size: 16px;
		}
	}

	&__od {
		margin-left: auto;
		color: #aaa;
		font-size: 11px;
		font-weight: 400;
	}
}

.eco-layer-row {
	display: flex;
	align-items: center;
	gap: 10px;
	padding: 8px 10px;
	background: var(--color-component-background);
	border-radius: 6px;
	cursor: pointer;
	transition: background 0.15s;

	&:hover {
		background: rgba(255, 255, 255, 0.04);
	}

	&__bar {
		width: 4px;
		height: 28px;
		border-radius: 2px;
		flex-shrink: 0;
	}

	&__icon-wrap {
		.material-icons-round {
			font-size: 18px;
		}
	}

	&__text {
		flex: 1;
		min-width: 0;
	}

	&__label {
		color: #fff;
		font-size: 13px;
		font-weight: 500;
	}

	&__meta {
		color: #888;
		font-size: 11px;
		margin-top: 2px;
	}

	&__switch {
		position: relative;
		width: 32px;
		height: 18px;
		background: #444;
		border-radius: 9px;
		flex-shrink: 0;
		cursor: pointer;
		transition: background 0.2s;

		&.is-on {
			background: #66bb6a;
		}
	}

	&__knob {
		position: absolute;
		top: 2px;
		left: 2px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: #fff;
		transition: transform 0.2s;

		.eco-layer-row__switch.is-on & {
			transform: translateX(14px);
		}
	}
}
</style>
