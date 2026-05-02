<!-- 顯示 plan_eco_route 結果 + 整合圖層開關 -->
<script setup>
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";

const props = defineProps({
	result: {
		type: Object,
		required: true,
	},
});

const router = useRouter();
const route = useRoute();
const store = useEcoAssistantStore();

const routes = computed(() => props.result?.routes || []);
const onMapView = computed(() => route.path === "/mapview");

// 計算「實際不重複」的景點數 (對應 EcoMapOverlay 的 dedup 邏輯)
const uniquePoiCount = computed(() => {
	const seen = new Set();
	for (const r of routes.value) {
		for (const p of r.green_points_passed || []) {
			seen.add(`${p.lat}|${p.lng}|${p.name}`);
		}
	}
	return seen.size;
});

function fmtKm(m) {
	return (m / 1000).toFixed(1);
}

function viewOnMap() {
	if (!onMapView.value) router.push("/mapview");
}

function poiIcon(type) {
	switch (type) {
		case "park": return "park";
		case "restaurant": return "restaurant";
		case "hotel": return "hotel";
		case "recycle": return "recycling";
		case "ubike": return "pedal_bike";
		default: return "place";
	}
}

function routeIcon(id) {
	switch (id) {
		case "shortest": return "bolt";
		case "balanced": return "balance";
		case "greenest": return "eco";
		default: return "alt_route";
	}
}
</script>

<template>
	<div class="eco-route-card">
		<header class="eco-route-card__header">
			<div class="eco-route-card__route">
				<div class="eco-route-card__endpoint">
					<span class="eco-route-card__dot eco-route-card__dot--start" />
					<span class="eco-route-card__endpoint-name">{{ result.start_name }}</span>
				</div>
				<span class="eco-route-card__arrow material-icons-round">arrow_downward</span>
				<div class="eco-route-card__endpoint">
					<span class="eco-route-card__dot eco-route-card__dot--end" />
					<span class="eco-route-card__endpoint-name">{{ result.end_name }}</span>
				</div>
			</div>
			<button
				class="eco-route-card__map-btn"
				:disabled="onMapView"
				:title="onMapView ? '已在地圖檢視' : '前往地圖檢視'"
				@click="viewOnMap"
			>
				<span class="material-icons-round">{{ onMapView ? "check_circle" : "map" }}</span>
				<span>{{ onMapView ? "已在地圖" : "在地圖檢視" }}</span>
			</button>
		</header>

		<ul class="eco-route-card__list">
			<li
				v-for="r in routes"
				:key="r.route_id"
				:class="[
					'eco-route',
					`eco-route--${r.route_id}`,
					{ 'eco-route--off': !store.layerVisibility[r.route_id] },
					{ 'eco-route--recommended': r.route_id === 'greenest' },
				]"
			>
				<div class="eco-route__head">
					<div class="eco-route__title-wrap">
						<span class="eco-route__icon material-icons-round">{{ routeIcon(r.route_id) }}</span>
						<span class="eco-route__title">{{ r.label }}</span>
						<span
							v-if="r.route_id === 'greenest'"
							class="eco-route__badge"
						>推薦</span>
					</div>
					<button
						class="eco-route__eye"
						:title="store.layerVisibility[r.route_id] ? '在地圖上隱藏' : '在地圖上顯示'"
						@click="store.toggleLayer(r.route_id)"
					>
						<span class="material-icons-round">
							{{ store.layerVisibility[r.route_id] ? "visibility" : "visibility_off" }}
						</span>
					</button>
				</div>
				<div class="eco-route__stats">
					<span class="eco-route__stat">
						<span class="material-icons-round">straighten</span>
						{{ fmtKm(r.distance_m) }} km
					</span>
					<span class="eco-route__stat">
						<span class="material-icons-round">schedule</span>
						{{ r.estimated_minutes }} 分
					</span>
					<span
						class="eco-route__stat eco-route__stat--green"
					>
						<span class="material-icons-round">eco</span>
						{{ r.green_points_passed.length }} 綠點
					</span>
				</div>
				<ul
					v-if="r.green_points_passed.length"
					class="eco-route__points"
				>
					<li
						v-for="p in r.green_points_passed.slice(0, 3)"
						:key="p.name"
					>
						<span class="material-icons-round">{{ poiIcon(p.type) }}</span>
						<span class="eco-route__point-name">{{ p.name }}</span>
					</li>
					<li
						v-if="r.green_points_passed.length > 3"
						class="eco-route__points-more"
					>
						+{{ r.green_points_passed.length - 3 }} 個景點
					</li>
				</ul>
			</li>
		</ul>

		<div class="eco-route-card__poi-toggle">
			<span class="material-icons-round eco-route-card__poi-icon">place</span>
			<span class="eco-route-card__poi-label">沿線景點</span>
			<span class="eco-route-card__poi-count">{{ uniquePoiCount }} 個</span>
			<button
				class="eco-route__eye"
				:title="store.layerVisibility.pois ? '隱藏景點' : '顯示景點'"
				@click="store.toggleLayer('pois')"
			>
				<span class="material-icons-round">
					{{ store.layerVisibility.pois ? "visibility" : "visibility_off" }}
				</span>
			</button>
		</div>
	</div>
</template>

<style scoped lang="scss">
.eco-route-card {
	margin-top: 12px;
	background: linear-gradient(180deg, #232b2c 0%, #1d2425 100%);
	border-radius: 14px;
	padding: 14px;
	border: 1px solid rgba(0, 184, 169, 0.25);
	box-shadow: 0 4px 14px rgba(0, 0, 0, 0.25);

	&__header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 10px;
		padding-bottom: 12px;
		margin-bottom: 12px;
		border-bottom: 1px dashed rgba(0, 184, 169, 0.18);
	}

	&__route {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		flex: 1;
	}

	&__endpoint {
		display: flex;
		align-items: center;
		gap: 7px;
		min-width: 0;
	}

	&__endpoint-name {
		font-size: 13px;
		font-weight: 600;
		color: #f1f1f1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	&__dot {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		flex: none;

		&--start {
			background: #81d8d0;
			box-shadow: 0 0 0 3px rgba(129, 216, 208, 0.18);
		}

		&--end {
			background: #ff8fb1;
			box-shadow: 0 0 0 3px rgba(255, 143, 177, 0.18);
		}
	}

	&__arrow {
		font-size: 14px !important;
		color: rgba(0, 184, 169, 0.55);
		margin-left: 1px;
	}

	&__map-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: linear-gradient(135deg, rgba(129, 216, 208, 0.18), rgba(0, 184, 169, 0.22));
		border: 1px solid rgba(0, 184, 169, 0.45);
		color: #a7ece5;
		border-radius: 999px;
		padding: 5px 12px;
		font-size: 11px;
		font-weight: 600;
		cursor: pointer;
		flex: none;
		transition: all 0.15s;
		white-space: nowrap;

		.material-icons-round {
			font-size: 14px;
		}

		&:hover:not(:disabled) {
			background: linear-gradient(135deg, rgba(129, 216, 208, 0.3), rgba(0, 184, 169, 0.35));
			color: #fff;
			transform: translateY(-1px);
		}

		&:disabled {
			opacity: 0.6;
			cursor: default;
			background: rgba(255, 255, 255, 0.05);
			border-color: rgba(255, 255, 255, 0.12);
			color: #9aa;
		}
	}

	&__list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	&__poi-toggle {
		margin-top: 12px;
		padding: 8px 10px;
		background: rgba(0, 184, 169, 0.06);
		border: 1px solid rgba(0, 184, 169, 0.18);
		border-radius: 10px;
		display: flex;
		align-items: center;
		gap: 7px;
		font-size: 12px;
		color: #ddd;
	}

	&__poi-icon {
		font-size: 16px !important;
		color: #00b8a9;
	}

	&__poi-label {
		font-weight: 600;
		flex: 1;
	}

	&__poi-count {
		background: rgba(0, 184, 169, 0.18);
		color: #a7ece5;
		font-size: 11px;
		font-weight: 600;
		padding: 2px 8px;
		border-radius: 999px;
		margin-right: 2px;
	}
}

.eco-route {
	position: relative;
	background: #1a2122;
	border-radius: 10px;
	padding: 10px 12px;
	border: 1px solid transparent;
	border-left: 3px solid #555;
	font-size: 13px;
	transition: all 0.2s ease;

	&--shortest { border-left-color: #ffb74d; }
	&--balanced { border-left-color: #4fc3f7; }
	&--greenest { border-left-color: #5dc07c; }

	&--recommended {
		background: linear-gradient(180deg, rgba(93, 192, 124, 0.12) 0%, #1a2122 100%);
		border-color: rgba(93, 192, 124, 0.35);
		border-left-color: #5dc07c;
	}

	&--off {
		opacity: 0.4;
	}

	&__head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		margin-bottom: 6px;
	}

	&__title-wrap {
		display: flex;
		align-items: center;
		gap: 6px;
		min-width: 0;
	}

	&__icon {
		font-size: 16px !important;
		color: #a7ece5;

		.eco-route--shortest & { color: #ffb74d; }
		.eco-route--balanced & { color: #4fc3f7; }
		.eco-route--greenest & { color: #5dc07c; }
	}

	&__title {
		font-weight: 700;
		color: #f5f5f5;
		font-size: 14px;
	}

	&__badge {
		font-size: 10px;
		font-weight: 700;
		color: #1a2122;
		background: linear-gradient(135deg, #5dc07c, #3fa85f);
		padding: 2px 7px;
		border-radius: 999px;
		letter-spacing: 0.5px;
	}

	&__eye {
		background: transparent;
		border: none;
		color: #888;
		cursor: pointer;
		padding: 4px;
		border-radius: 6px;
		display: flex;
		align-items: center;
		flex: none;
		transition: all 0.15s;

		.material-icons-round {
			font-size: 16px;
		}

		&:hover {
			color: #fff;
			background: rgba(255, 255, 255, 0.06);
		}
	}

	&__stats {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}

	&__stat {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: 11px;
		color: #c5c5c5;
		background: rgba(255, 255, 255, 0.05);
		padding: 3px 8px;
		border-radius: 999px;

		.material-icons-round {
			font-size: 12px !important;
			color: #999;
		}

		&--green {
			background: rgba(93, 192, 124, 0.15);
			color: #c0e8c8;

			.material-icons-round {
				color: #5dc07c;
			}
		}
	}

	&__points {
		list-style: none;
		padding: 8px 0 0;
		margin: 8px 0 0;
		border-top: 1px dashed rgba(255, 255, 255, 0.08);
		display: flex;
		flex-direction: column;
		gap: 4px;

		li {
			display: flex;
			align-items: center;
			gap: 6px;
			color: #d0d0d0;
			font-size: 12px;

			.material-icons-round {
				font-size: 14px;
				color: #5dc07c;
			}
		}
	}

	&__point-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	&__points-more {
		color: #888 !important;
		font-size: 11px;
		font-style: italic;
		padding-left: 20px;
	}
}
</style>
