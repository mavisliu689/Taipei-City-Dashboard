<!-- 顯示走路 vs 其他交通的減碳量, 取代過去的純文字 buildCarbonSavingReply -->
<script setup>
import { computed } from "vue";

const props = defineProps({
	payload: {
		type: Object,
		required: true,
	},
});

const routes = computed(() => props.payload?.routes || []);

function modeIcon(key) {
	switch (key) {
		case "car": return "directions_car";
		case "scooter": return "two_wheeler";
		case "bus": return "directions_bus";
		case "mrt": return "train";
		default: return "commute";
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
	<div class="eco-carbon-card">
		<header class="eco-carbon-card__header">
			<span class="material-icons-round eco-carbon-card__leaf">eco</span>
			<div class="eco-carbon-card__title-wrap">
				<div class="eco-carbon-card__title">走路減碳量</div>
				<div class="eco-carbon-card__route-line">
					<span class="eco-carbon-card__endpoint">{{ payload.startName }}</span>
					<span class="material-icons-round eco-carbon-card__arrow">arrow_right_alt</span>
					<span class="eco-carbon-card__endpoint">{{ payload.endName }}</span>
				</div>
			</div>
		</header>

		<ul class="eco-carbon-card__list">
			<li
				v-for="r in routes"
				:key="r.id"
				:class="['eco-carbon-route', `eco-carbon-route--${r.id}`]"
			>
				<div class="eco-carbon-route__head">
					<span class="material-icons-round eco-carbon-route__icon">{{ routeIcon(r.id) }}</span>
					<span class="eco-carbon-route__label">{{ r.label }}</span>
					<span class="eco-carbon-route__km">{{ r.km.toFixed(1) }} km</span>
				</div>

				<div
					v-if="r.hero"
					class="eco-carbon-hero"
				>
					<div class="eco-carbon-hero__lhs">
						<span class="material-icons-round">{{ modeIcon(r.hero.key) }}</span>
						<span class="eco-carbon-hero__lhs-label">vs {{ r.hero.label }}</span>
					</div>
					<div class="eco-carbon-hero__rhs">
						<div class="eco-carbon-hero__big">
							{{ r.hero.savedKg.toFixed(2) }}
							<span class="eco-carbon-hero__unit">kg CO₂e</span>
						</div>
						<div class="eco-carbon-hero__sub">
							<span class="material-icons-round">park</span>
							≈ {{ r.hero.trees.toFixed(2) }} 棵樹一年固碳
						</div>
					</div>
				</div>

				<ul
					v-if="r.baselines.length > 1"
					class="eco-carbon-rows"
				>
					<li
						v-for="b in r.baselines.slice(1)"
						:key="b.key"
						class="eco-carbon-row"
					>
						<span class="material-icons-round eco-carbon-row__icon">{{ modeIcon(b.key) }}</span>
						<span class="eco-carbon-row__mode">{{ b.label }}</span>
						<span class="eco-carbon-row__kg">{{ b.savedKg.toFixed(2) }} kg</span>
						<span class="eco-carbon-row__sep">·</span>
						<span class="eco-carbon-row__trees">{{ b.trees.toFixed(2) }} 樹</span>
					</li>
				</ul>
			</li>
		</ul>

		<footer class="eco-carbon-card__footer">
			<span class="material-icons-round">info</span>
			<span>排放因子：環境部、北捷年報；樹木年固碳 21.77 kg/棵（林業署）</span>
		</footer>
	</div>
</template>

<style scoped lang="scss">
.eco-carbon-card {
	margin-top: 12px;
	background: linear-gradient(180deg, #232b2c 0%, #1d2425 100%);
	border-radius: 14px;
	padding: 14px;
	border: 1px solid rgba(93, 192, 124, 0.28);
	box-shadow: 0 4px 14px rgba(0, 0, 0, 0.25);

	&__header {
		display: flex;
		align-items: flex-start;
		gap: 9px;
		padding-bottom: 12px;
		margin-bottom: 12px;
		border-bottom: 1px dashed rgba(93, 192, 124, 0.2);
	}

	&__leaf {
		font-size: 22px !important;
		color: #5dc07c;
		flex: none;
		margin-top: 2px;
	}

	&__title-wrap {
		flex: 1;
		min-width: 0;
	}

	&__title {
		font-weight: 700;
		color: #f1f1f1;
		font-size: 15px;
		margin-bottom: 4px;
	}

	&__route-line {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: #c5c5c5;
		min-width: 0;
		flex-wrap: wrap;
	}

	&__endpoint {
		font-weight: 600;
		color: #e0e0e0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 140px;
	}

	&__arrow {
		font-size: 16px !important;
		color: #5dc07c;
	}

	&__list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	&__footer {
		margin-top: 12px;
		padding-top: 10px;
		border-top: 1px dashed rgba(255, 255, 255, 0.08);
		display: flex;
		align-items: flex-start;
		gap: 6px;
		font-size: 11px;
		color: #888;
		line-height: 1.5;

		.material-icons-round {
			font-size: 13px !important;
			color: #666;
			margin-top: 1px;
			flex: none;
		}
	}
}

.eco-carbon-route {
	background: #1a2122;
	border-radius: 10px;
	padding: 10px 12px;
	border-left: 3px solid #555;
	border: 1px solid transparent;
	border-left-width: 3px;

	&--shortest { border-left-color: #ffb74d; }
	&--balanced { border-left-color: #4fc3f7; }
	&--greenest { border-left-color: #5dc07c; }

	&__head {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-bottom: 8px;
	}

	&__icon {
		font-size: 16px !important;
		color: #a7ece5;

		.eco-carbon-route--shortest & { color: #ffb74d; }
		.eco-carbon-route--balanced & { color: #4fc3f7; }
		.eco-carbon-route--greenest & { color: #5dc07c; }
	}

	&__label {
		font-weight: 700;
		color: #f5f5f5;
		font-size: 13px;
		flex: 1;
	}

	&__km {
		font-size: 11px;
		color: #aaa;
		background: rgba(255, 255, 255, 0.05);
		padding: 2px 8px;
		border-radius: 999px;
	}
}

.eco-carbon-hero {
	display: flex;
	align-items: center;
	gap: 10px;
	background: linear-gradient(135deg, rgba(93, 192, 124, 0.12), rgba(93, 192, 124, 0.04));
	border: 1px solid rgba(93, 192, 124, 0.25);
	border-radius: 10px;
	padding: 10px 12px;

	&__lhs {
		display: flex;
		align-items: center;
		gap: 5px;
		flex: none;

		.material-icons-round {
			font-size: 18px !important;
			color: #c0e8c8;
		}
	}

	&__lhs-label {
		font-size: 12px;
		color: #c0e8c8;
		font-weight: 600;
	}

	&__rhs {
		flex: 1;
		min-width: 0;
		text-align: right;
	}

	&__big {
		font-size: 22px;
		font-weight: 800;
		color: #5dc07c;
		line-height: 1.1;
		letter-spacing: -0.5px;
	}

	&__unit {
		font-size: 12px;
		font-weight: 600;
		color: #a7ece5;
		margin-left: 4px;
	}

	&__sub {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: 11px;
		color: #b0c8b6;
		margin-top: 2px;

		.material-icons-round {
			font-size: 12px !important;
			color: #5dc07c;
		}
	}
}

.eco-carbon-rows {
	list-style: none;
	padding: 0;
	margin: 8px 0 0;
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.eco-carbon-row {
	display: flex;
	align-items: center;
	gap: 7px;
	padding: 4px 8px;
	font-size: 12px;
	color: #c5c5c5;

	&__icon {
		font-size: 14px !important;
		color: #888;
		flex: none;
	}

	&__mode {
		flex: none;
		min-width: 36px;
		color: #ccc;
	}

	&__kg {
		flex: 1;
		font-weight: 600;
		color: #e0e0e0;
		font-variant-numeric: tabular-nums;
	}

	&__sep {
		color: #555;
	}

	&__trees {
		color: #999;
		font-variant-numeric: tabular-nums;
	}
}
</style>
