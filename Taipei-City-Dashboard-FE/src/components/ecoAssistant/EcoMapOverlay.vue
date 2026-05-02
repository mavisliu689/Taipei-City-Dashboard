<!--
  EcoMapOverlay — 行為元件 (template 空)，把減碳助手回傳的 3 條路線
  + 沿途景點 + 起終點 marker 推到主 mapStore.map。

  UI 控制 (圖層開關、地圖點選按鈕) 已搬到 EcoRouteCard.vue / MapPickerControl.vue。
  此元件純行為：渲染、清理、地圖事件處理。
-->
<script setup>
import mapboxGl from "mapbox-gl";
import { onBeforeUnmount, watch } from "vue";

import { fetchWalkingRoute } from "../../assets/utilityFunctions/mapboxDirections.js";
import { useEcoAssistantStore } from "../../store/ecoAssistantStore.js";
import { useMapStore } from "../../store/mapStore.js";
import { Geocode_dictionary } from "./geocodeDictionary.js";

const ecoStore = useEcoAssistantStore();
const mapStore = useMapStore();

const SRC_ROUTES = "eco-route-routes";
const SRC_POIS = "eco-route-pois";
const LAYER_LINE_GLOW = "eco-route-line-glow";
const LAYER_LINE = "eco-route-line";
const LAYER_POI_HALO = "eco-route-poi-halo";
const LAYER_POI_CIRCLE = "eco-route-poi-circle";
const LAYER_POI_LABEL = "eco-route-poi-label";

const ROUTE_META = {
	shortest: { color: "#ffb74d" },
	balanced: { color: "#4fc3f7" },
	greenest: { color: "#66bb6a" },
};

const POI_TYPE_COLORS = {
	park: "#66bb6a",
	restaurant: "#ff8a65",
	hotel: "#ba68c8",
	recycle: "#fdd835",
	ubike: "#26a69a",
};

const POI_TYPE_ICONS = {
	park: "park",
	restaurant: "restaurant",
	hotel: "hotel",
	recycle: "recycling",
	ubike: "pedal_bike",
};

const POI_TYPE_LABELS = {
	park: "公園綠地",
	restaurant: "環保餐廳",
	hotel: "環保旅館",
	recycle: "回收站",
	ubike: "YouBike 站點",
};

const POI_ROW_ICONS = {
	address: "place",
	district: "map",
	city: "location_city",
};

function escapeHtml(s) {
	if (s == null) return "";
	return String(s).replace(/[&<>"']/g, (c) => ({
		"&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;",
	}[c]));
}

function buildPopupHTML(p, type) {
	const cat = POI_TYPE_LABELS[type] || type || "POI";
	const accent = POI_TYPE_COLORS[type] || "#66bb6a";
	const rows = [];
	if (p.address) rows.push(["address", p.address]);
	if (p.district) rows.push(["district", p.district]);
	if (p.city && !p.district) rows.push(["city", p.city]);
	const rowsHtml = rows
		.map(([k, v]) => `
			<div class="eco-pop__row">
				<span class="material-icons-round eco-pop__row-icon">${POI_ROW_ICONS[k] || "info"}</span>
				<span class="eco-pop__row-value">${escapeHtml(v)}</span>
			</div>`)
		.join("");
	return `
		<div class="eco-pop" style="--accent:${accent};">
			<div class="eco-pop__cat">${escapeHtml(cat)}</div>
			<div class="eco-pop__name">${escapeHtml(p.name || "(未命名)")}</div>
			${rows.length ? `<div class="eco-pop__rows">${rowsHtml}</div>` : ""}
		</div>
	`;
}

// 由 store 提供的 visibility，與 RouteCard UI 共享
const visibility = ecoStore.layerVisibility;

// ==================== 起終點座標解析 ====================
function resolveOrigin(result) {
	return result?.start_coord || Geocode_dictionary[result?.start_name] || null;
}
function resolveDest(result) {
	return result?.end_coord || Geocode_dictionary[result?.end_name] || null;
}

// ==================== 路線 / POI feature 建構 ====================
function waypointsForRoute(route, origin, dest) {
	return [
		[origin.lng, origin.lat],
		...route.green_points_passed.map((p) => [p.lng, p.lat]),
		[dest.lng, dest.lat],
	];
}

async function buildRouteFeatures(result) {
	const origin = resolveOrigin(result);
	const dest = resolveDest(result);
	if (!origin || !dest) return [];
	const tasks = result.routes.map(async (r) => {
		let coords;
		if (r.geometry?.coordinates?.length) {
			coords = r.geometry.coordinates;
		} else {
			const wp = waypointsForRoute(r, origin, dest);
			const real = await fetchWalkingRoute(wp);
			coords = real?.geometry?.coordinates || wp;
		}
		return {
			type: "Feature",
			properties: {
				route_id: r.route_id,
				distance_m: r.distance_m,
				estimated_minutes: r.estimated_minutes,
			},
			geometry: { type: "LineString", coordinates: coords },
		};
	});
	return Promise.all(tasks);
}

// 起終點不再用 style layer，改用 ensureManualMarkers 的可拖曳 DOM marker
function buildPOIsGeoJSON() {
	return { type: "FeatureCollection", features: [] };
}

// ==================== 地圖 layers ====================
function removeLayers(map) {
	for (const id of [LAYER_POI_LABEL, LAYER_POI_CIRCLE, LAYER_POI_HALO, LAYER_LINE, LAYER_LINE_GLOW]) {
		if (map.getLayer(id)) map.removeLayer(id);
	}
	for (const id of [SRC_POIS, SRC_ROUTES]) {
		if (map.getSource(id)) map.removeSource(id);
	}
	removePoiMarkers();
	// 搜尋結果 marker 不在這邊清除 (跟 route 是不同生命週期)
}

function addAllLayers(map, routesGeoJSON, poisGeoJSON) {
	map.addSource(SRC_ROUTES, { type: "geojson", data: routesGeoJSON });
	const colorExpr = [
		"match", ["get", "route_id"],
		"shortest", ROUTE_META.shortest.color,
		"balanced", ROUTE_META.balanced.color,
		"greenest", ROUTE_META.greenest.color,
		"#888888",
	];
	map.addLayer({
		id: LAYER_LINE_GLOW,
		type: "line",
		source: SRC_ROUTES,
		paint: {
			"line-color": colorExpr,
			"line-width": ["match", ["get", "route_id"], "greenest", 14, "balanced", 11, "shortest", 9, 9],
			"line-opacity": 0.18,
			"line-blur": 3,
		},
		layout: { "line-cap": "round", "line-join": "round" },
	});
	map.addLayer({
		id: LAYER_LINE,
		type: "line",
		source: SRC_ROUTES,
		paint: {
			"line-color": colorExpr,
			"line-width": ["match", ["get", "route_id"], "greenest", 5, "balanced", 4, "shortest", 3, 3],
			"line-opacity": 0.95,
		},
		layout: { "line-cap": "round", "line-join": "round" },
	});

	map.addSource(SRC_POIS, { type: "geojson", data: poisGeoJSON });
	// 起終點仍用 style layer (origin/destination 兩個白色大圓)
	map.addLayer({
		id: LAYER_POI_HALO,
		type: "circle",
		source: SRC_POIS,
		paint: {
			"circle-radius": 16,
			"circle-color": "#ffffff",
			"circle-opacity": 0.25,
			"circle-blur": 0.4,
		},
	});
	map.addLayer({
		id: LAYER_POI_CIRCLE,
		type: "circle",
		source: SRC_POIS,
		paint: {
			"circle-radius": 8,
			"circle-color": "#ffffff",
			"circle-stroke-width": 3,
			"circle-stroke-color": "#1b5e20",
		},
	});
	map.addLayer({
		id: LAYER_POI_LABEL,
		type: "symbol",
		source: SRC_POIS,
		layout: {
			"text-field": ["get", "name"],
			"text-size": 12,
			"text-offset": [0, 1.5],
			"text-anchor": "top",
			"text-allow-overlap": false,
			"text-font": ["Open Sans Regular", "Arial Unicode MS Regular"],
		},
		paint: {
			"text-color": "#ffffff",
			"text-halo-color": "#1b5e20",
			"text-halo-width": 1.5,
		},
	});
}

function fitBounds(map, features) {
	const all = [];
	for (const f of features) all.push(...f.geometry.coordinates);
	if (!all.length) return;
	const lngs = all.map((c) => c[0]);
	const lats = all.map((c) => c[1]);
	map.fitBounds(
		[
			[Math.min(...lngs), Math.min(...lats)],
			[Math.max(...lngs), Math.max(...lats)],
		],
		{ padding: 100, duration: 800 }
	);
}

function applyVisibilityFilter(map) {
	if (map.getLayer(LAYER_LINE)) {
		const visibleRoutes = ["shortest", "balanced", "greenest"].filter((k) => visibility[k]);
		const filter = ["in", ["get", "route_id"], ["literal", visibleRoutes]];
		map.setFilter(LAYER_LINE, filter);
		map.setFilter(LAYER_LINE_GLOW, filter);
	}
}

// ==================== POI markers (DOM 元素，視覺穩定) ====================
let poiMarkers = [];
let searchPoiMarkers = []; // find_eco_pois 結果

function makePoiMarkerEl(type, name) {
	const color = POI_TYPE_COLORS[type] || "#888";
	const icon = POI_TYPE_ICONS[type] || "place";
	const wrapper = document.createElement("div");
	// pointer-events:auto + cursor:pointer 讓 hover/click 可被 marker 捕捉
	wrapper.style.cssText = "display:flex;flex-direction:column;align-items:center;pointer-events:auto;cursor:pointer;";
	const circle = document.createElement("div");
	circle.style.cssText = `
		width: 30px; height: 30px; border-radius: 50%;
		background: ${color}; color: white;
		border: 3px solid white; box-shadow: 0 2px 8px rgba(0,0,0,0.6);
		display: flex; align-items: center; justify-content: center;
		font-family: 'Material Icons Round'; font-size: 16px;
	`;
	circle.textContent = icon;
	const label = document.createElement("div");
	label.style.cssText = `
		margin-top: 3px; padding: 2px 6px; border-radius: 4px;
		background: rgba(27, 94, 32, 0.92); color: #fff;
		font-size: 11px; white-space: nowrap;
		max-width: 140px; overflow: hidden; text-overflow: ellipsis;
		font-weight: 500;
	`;
	label.textContent = name;
	wrapper.appendChild(circle);
	wrapper.appendChild(label);
	return wrapper;
}

// hover popup (跟隨滑鼠進出, 隨時換 marker), pinned popup (click 釘住)
let hoverPopup = null;
let pinnedPopup = null;

function ensurePopupInstances() {
	if (!hoverPopup) {
		hoverPopup = new mapboxGl.Popup({
			closeButton: false,
			closeOnClick: false,
			offset: 26,
			className: "eco-poi-popup",
			maxWidth: "260px",
		});
	}
	if (!pinnedPopup) {
		pinnedPopup = new mapboxGl.Popup({
			closeButton: true,
			closeOnClick: false,
			offset: 26,
			className: "eco-poi-popup eco-poi-popup--pinned",
			maxWidth: "260px",
		});
	}
}

function attachPoiInteractivity(markerEl, marker, poi, type, map) {
	ensurePopupInstances();
	markerEl.addEventListener("mouseenter", () => {
		// 已釘住同一點不再開 hover (避免閃爍)
		if (pinnedPopup.isOpen() && pinnedPopup._lngLat
			&& pinnedPopup._lngLat.lng === marker.getLngLat().lng
			&& pinnedPopup._lngLat.lat === marker.getLngLat().lat) return;
		hoverPopup
			.setLngLat(marker.getLngLat())
			.setHTML(buildPopupHTML(poi, type))
			.addTo(map);
	});
	markerEl.addEventListener("mouseleave", () => {
		hoverPopup.remove();
	});
	markerEl.addEventListener("click", (e) => {
		e.stopPropagation();
		hoverPopup.remove();
		const ll = marker.getLngLat();
		const samePinned = pinnedPopup.isOpen() && pinnedPopup._lngLat
			&& pinnedPopup._lngLat.lng === ll.lng
			&& pinnedPopup._lngLat.lat === ll.lat;
		if (samePinned) {
			pinnedPopup.remove();
		} else {
			pinnedPopup
				.setLngLat(ll)
				.setHTML(buildPopupHTML(poi, type))
				.addTo(map);
		}
	});
}

function removeAllPopups() {
	if (hoverPopup) hoverPopup.remove();
	if (pinnedPopup) pinnedPopup.remove();
}

function removePoiMarkers() {
	for (const m of poiMarkers) m.remove();
	poiMarkers = [];
	removeAllPopups();
}

function ensurePoiMarkers(map, result) {
	removePoiMarkers();
	if (!visibility.pois) return;
	const seen = new Set();
	for (const r of result.routes) {
		for (const p of r.green_points_passed) {
			const key = `${p.lat}|${p.lng}|${p.name}`;
			if (seen.has(key)) continue;
			seen.add(key);
			const el = makePoiMarkerEl(p.type, p.name);
			const m = new mapboxGl.Marker({ element: el })
				.setLngLat([p.lng, p.lat])
				.addTo(map);
			attachPoiInteractivity(el, m, p, p.type, map);
			poiMarkers.push(m);
		}
	}
}

function removeSearchPoiMarkers() {
	for (const m of searchPoiMarkers) m.remove();
	searchPoiMarkers = [];
	removeAllPopups();
}

function ensureSearchPoiMarkers(map, search) {
	removeSearchPoiMarkers();
	if (!visibility.search || !search?.items?.length) return;
	const visibleBounds = map.getBounds();
	let minLng = Infinity, minLat = Infinity, maxLng = -Infinity, maxLat = -Infinity;
	for (const p of search.items) {
		if (p.lat == null || p.lng == null) continue;
		const el = makePoiMarkerEl(p.category, p.name);
		const m = new mapboxGl.Marker({ element: el })
			.setLngLat([p.lng, p.lat])
			.addTo(map);
		attachPoiInteractivity(el, m, p, p.category, map);
		searchPoiMarkers.push(m);
		minLng = Math.min(minLng, p.lng);
		minLat = Math.min(minLat, p.lat);
		maxLng = Math.max(maxLng, p.lng);
		maxLat = Math.max(maxLat, p.lat);
	}
	// 若搜尋結果都在目前視野外, 自動 fitBounds
	if (searchPoiMarkers.length && !visibleBounds.contains([(minLng + maxLng) / 2, (minLat + maxLat) / 2])) {
		map.fitBounds([[minLng, minLat], [maxLng, maxLat]], { padding: 80, duration: 600 });
	}
}

// ==================== Render flow ====================
const cached = { result: null, routeFeatures: null };

async function renderRoute(result) {
	const map = mapStore.map;
	if (!map || !result) return;
	if (!map.isStyleLoaded()) {
		map.once("style.load", () => renderRoute(result));
		return;
	}
	const routeFeatures = await buildRouteFeatures(result);
	if (!routeFeatures.length) return;
	cached.result = result;
	cached.routeFeatures = routeFeatures;
	const routesGeoJSON = { type: "FeatureCollection", features: routeFeatures };
	const poisGeoJSON = buildPOIsGeoJSON(result);
	removeLayers(map);
	addAllLayers(map, routesGeoJSON, poisGeoJSON);
	ensurePoiMarkers(map, result);
	applyVisibilityFilter(map);
	// 起終點同步寫進 manualOrigin/manualDest -> ensureManualMarkers 會建立
	// 可拖曳的彩色 marker (跟 picker 完全一致), 拖曳後自動 re-plan
	const o = resolveOrigin(result);
	const d = resolveDest(result);
	if (o) ecoStore.setManualOrigin({ lat: o.lat, lng: o.lng }, { silent: true });
	if (d) ecoStore.setManualDest({ lat: d.lat, lng: d.lng }, { silent: true });
	ensureManualMarkers(map);
	fitBounds(map, routeFeatures);
}

function clearRoute() {
	const map = mapStore.map;
	if (map) removeLayers(map);
	cached.result = null;
	cached.routeFeatures = null;
}

// ==================== 起終點 manual marker (拖曳 / 點選) ====================
let originMarker = null;
let destMarker = null;
let mapClickHandler = null;
let cursorWatcher = null;
let attachedToMap = null; // 已綁定 click handler 的 map 物件

function makeManualMarkerEl(color, icon) {
	const el = document.createElement("div");
	el.style.cssText = `
		width: 30px; height: 30px; border-radius: 50%;
		background: ${color}; color: white; border: 3px solid white;
		box-shadow: 0 2px 8px rgba(0,0,0,0.6);
		display: flex; align-items: center; justify-content: center;
		font-family: 'Material Icons Round'; font-size: 18px; cursor: grab;
	`;
	el.textContent = icon;
	return el;
}

function ensureManualMarkers(map) {
	// origin
	if (ecoStore.manualOrigin) {
		const ll = [ecoStore.manualOrigin.lng, ecoStore.manualOrigin.lat];
		if (!originMarker) {
			originMarker = new mapboxGl.Marker({ element: makeManualMarkerEl("#4fc3f7", "trip_origin"), draggable: true })
				.setLngLat(ll)
				.addTo(map);
			originMarker.on("dragend", () => {
				const c = originMarker.getLngLat();
				ecoStore.setManualOrigin({ lat: c.lat, lng: c.lng });
			});
		} else {
			originMarker.setLngLat(ll);
		}
	} else if (originMarker) {
		originMarker.remove();
		originMarker = null;
	}
	// dest
	if (ecoStore.manualDest) {
		const ll = [ecoStore.manualDest.lng, ecoStore.manualDest.lat];
		if (!destMarker) {
			destMarker = new mapboxGl.Marker({ element: makeManualMarkerEl("#ff8a65", "place"), draggable: true })
				.setLngLat(ll)
				.addTo(map);
			destMarker.on("dragend", () => {
				const c = destMarker.getLngLat();
				ecoStore.setManualDest({ lat: c.lat, lng: c.lng });
			});
		} else {
			destMarker.setLngLat(ll);
		}
	} else if (destMarker) {
		destMarker.remove();
		destMarker = null;
	}
}

function attachMapClickOnce(map) {
	if (attachedToMap === map) return;
	// 清掉舊的 (若 map 替換)
	detachFromCurrentMap();
	mapClickHandler = (e) => {
		if (ecoStore.pickMode === "idle") return;
		const coord = { lat: e.lngLat.lat, lng: e.lngLat.lng };
		if (ecoStore.pickMode === "origin") {
			ecoStore.setManualOrigin(coord);
		} else if (ecoStore.pickMode === "destination") {
			ecoStore.setManualDest(coord);
		}
		ecoStore.setPickMode("idle"); // 點完自動退出
	};
	map.on("click", mapClickHandler);
	cursorWatcher = watch(
		() => ecoStore.pickMode,
		(m) => {
			const c = map.getCanvas();
			if (c) c.style.cursor = m === "idle" ? "" : "crosshair";
		},
		{ immediate: true }
	);
	attachedToMap = map;
}

function detachFromCurrentMap() {
	if (attachedToMap && mapClickHandler) {
		attachedToMap.off("click", mapClickHandler);
		const c = attachedToMap.getCanvas?.();
		if (c) c.style.cursor = "";
	}
	if (cursorWatcher) cursorWatcher();
	mapClickHandler = null;
	cursorWatcher = null;
	attachedToMap = null;
}

function cleanupAllManualMarkers() {
	if (originMarker) {
		originMarker.remove();
		originMarker = null;
	}
	if (destMarker) {
		destMarker.remove();
		destMarker = null;
	}
}

// ==================== Watchers ====================
watch(
	() => ecoStore.currentRoute,
	(route) => {
		if (route) renderRoute(route);
		else clearRoute();
	},
	{ immediate: true }
);

watch(
	() => mapStore.map,
	(map) => {
		if (map) {
			attachMapClickOnce(map);
			ensureManualMarkers(map);
			if (ecoStore.currentRoute) renderRoute(ecoStore.currentRoute);
		} else {
			detachFromCurrentMap();
			cleanupAllManualMarkers();
		}
	},
	{ immediate: true }
);

watch(
	() => [ecoStore.manualOrigin, ecoStore.manualDest],
	() => {
		if (mapStore.map) ensureManualMarkers(mapStore.map);
	},
	{ deep: true }
);

// 監聽 store 圖層 visibility -> 重新套用 filter / POI markers
watch(
	() => ({ ...ecoStore.layerVisibility }),
	(newV, oldV) => {
		const map = mapStore.map;
		if (!map) return;
		applyVisibilityFilter(map);
		if (oldV && oldV.pois !== newV.pois && cached.result) {
			if (newV.pois) ensurePoiMarkers(map, cached.result);
			else removePoiMarkers();
		}
		if (oldV && oldV.search !== newV.search && ecoStore.currentSearchPOIs) {
			if (newV.search) ensureSearchPoiMarkers(map, ecoStore.currentSearchPOIs);
			else removeSearchPoiMarkers();
		}
	},
	{ deep: true }
);

// find_eco_pois 結果 -> 渲染搜尋 marker
watch(
	() => ecoStore.currentSearchPOIs,
	(search) => {
		const map = mapStore.map;
		if (!map) return;
		if (search) ensureSearchPoiMarkers(map, search);
		else removeSearchPoiMarkers();
	},
	{ immediate: true }
);

onBeforeUnmount(() => {
	clearRoute();
	cleanupAllManualMarkers();
	removeSearchPoiMarkers();
	detachFromCurrentMap();
});
</script>

<template>
	<!-- 純行為元件，不渲染 DOM (UI 已搬到 RouteCard / MapPickerControl) -->
</template>

<style lang="scss">
// 非 scoped — mapbox popup 注入到 .mapboxgl-popup 容器, scoped 樣式選不到
.eco-poi-popup {
	z-index: 12;

	// 隱藏預設箭頭, 自己畫 — 預設 tip 顏色蓋不掉漸層
	.mapboxgl-popup-tip {
		display: none;
	}

	.mapboxgl-popup-content {
		padding: 0;
		background: transparent;
		border: none;
		box-shadow: none;
		filter: drop-shadow(0 14px 28px rgba(0, 0, 0, 0.55))
			drop-shadow(0 4px 10px rgba(0, 0, 0, 0.35));
		// 讓 popup 浮現有微淡入動效
		animation: ecoPopFadeIn 140ms ease-out both;
	}

	.mapboxgl-popup-close-button {
		color: #b9f6ca;
		font-size: 18px;
		line-height: 1;
		padding: 2px 8px;
		right: 6px;
		top: 6px;
		border-radius: 8px;
		background: rgba(0, 0, 0, 0.35);
		border: 1px solid rgba(255, 255, 255, 0.08);
		z-index: 2;
		transition: background 0.15s, color 0.15s;

		&:hover {
			background: rgba(102, 187, 106, 0.28);
			color: #fff;
		}
	}
}

@keyframes ecoPopFadeIn {
	from { opacity: 0; transform: translateY(4px); }
	to   { opacity: 1; transform: translateY(0); }
}

.eco-pop {
	font-family: inherit;
	color: #f1faf3;
	min-width: 200px;
	max-width: 260px;
	padding: 12px 14px;
	border-radius: 12px;
	background: rgba(22, 28, 26, 0.94);
	backdrop-filter: blur(10px);
	-webkit-backdrop-filter: blur(10px);

	&__cat {
		display: inline-block;
		padding: 2px 9px;
		border-radius: 999px;
		background: color-mix(in srgb, var(--accent) 18%, transparent);
		color: color-mix(in srgb, var(--accent) 30%, #ffffff);
		font-size: 10.5px;
		font-weight: 700;
		letter-spacing: 0.3px;
		white-space: nowrap;
		margin-bottom: 8px;
	}

	&__name {
		font-size: 15px;
		font-weight: 700;
		color: #fff;
		line-height: 1.35;
		letter-spacing: 0.2px;
		word-break: break-word;
		margin-bottom: 2px;
	}

	&__rows {
		margin-top: 10px;
		padding-top: 10px;
		border-top: 1px solid rgba(255, 255, 255, 0.08);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	&__row {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		font-size: 12.5px;
		line-height: 1.45;
	}

	&__row-icon {
		font-size: 14px !important;
		color: color-mix(in srgb, var(--accent) 55%, #ffffff 45%);
		opacity: 0.9;
		margin-top: 2px;
		flex: none;
	}

	&__row-value {
		color: #d8e8d9;
		word-break: break-word;
		flex: 1;
		min-width: 0;
	}
}
</style>
