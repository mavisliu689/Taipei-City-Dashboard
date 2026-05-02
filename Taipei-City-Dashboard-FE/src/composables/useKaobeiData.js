// 黑客松「靠北儀表板」用：呼叫 6 支 BE API（同 envelope: { data, status, total }），
// 把 raw rows 轉成 ComponentConfig.chart_data + 各圖層的 FeatureCollection cache。
// contentStore 在 setCurrentDashboardAllContent 偵測 index="kaobei" 時直接呼叫 loadKaobeiComponents()；
// mapStore.fetchLocalGeoJson 對 kaobei_* prefix 的圖層呼叫 getKaobeiGeoJson(index) 從 cache 取。
//
// 雙北/臺北市 dropdown filter：每個組件「獨立」一個 city state，切某組件 dropdown
// 只重打那一支 BE API（不從 cache 讀，永遠真實 fetch）。其他 5 個組件不動。

import axios from "axios";
import { reactive } from "vue";
import { useAuthStore } from "../store/authStore";
import { CHART_TOKENS, UBIKE_AREA_PALETTE, GREEN_RAMP } from "./chartTokens";

const CITY = "metrotaipei";

// FE 內部值 → BE query string（BE 認得 both / origin / new）
const CITY_TO_BE_QUERY = {
	metrotaipei: "both",
	taipei:      "origin",
	newtaipei:   "new",
};

// Dropdown 選項（給 view 端 :select-btn-list 用）
export const KAOBEI_CITY_OPTIONS = [
	{ value: "metrotaipei", name: "雙北" },
	{ value: "taipei",      name: "臺北市" },
	{ value: "newtaipei",   name: "新北市" },
];

// 不支援新北市的組件（目前已無；保留 set 給未來可能的 BE 限制 component 用）
const KAOBEI_NEWTAIPEI_UNSUPPORTED = new Set();

// kaobei dashboard 5 組件的 index 集合（給 view 端 listener 判斷是否走 kaobei 分支）
// 親子步道 (hiking_trail) 已移除
export const KAOBEI_COMPONENT_INDICES = new Set([
	"park_greenspace",
	"eco_restaurant",
	"eco_hotel",
	"recycle_station",
	"youbike_availability",
]);

// component index ↔ resource key 對映（FETCH/TRANSFORMS/FC_BUILDERS 的 key）
const INDEX_TO_RESOURCE = {
	park_greenspace:      "parks",
	eco_restaurant:       "restaurant",
	eco_hotel:            "hotel",
	recycle_station:      "recycle",
	youbike_availability: "ubike",
};

// 每個組件獨立的 city state（reactive object）。F5 / module 重載時重置回 metrotaipei
export const kaobeiCityFilters = reactive({
	park_greenspace:      "metrotaipei",
	eco_restaurant:       "metrotaipei",
	eco_hotel:            "metrotaipei",
	recycle_station:      "metrotaipei",
	youbike_availability: "metrotaipei",
});

export function getKaobeiCityFilter(componentIndex) {
	return kaobeiCityFilters[componentIndex];
}

export function getKaobeiCityOptions(componentIndex) {
	if (KAOBEI_NEWTAIPEI_UNSUPPORTED.has(componentIndex)) {
		return KAOBEI_CITY_OPTIONS.filter((option) => option.value !== "newtaipei");
	}
	return KAOBEI_CITY_OPTIONS;
}

// 進 kaobei dashboard 時呼叫：重置 6 個組件的 dropdown 回「雙北」（PLAN §6 改版需求）
export function resetKaobeiCityFilters() {
	Object.keys(kaobeiCityFilters).forEach((k) => {
		kaobeiCityFilters[k] = "metrotaipei";
	});
}

// 端點：baseURL = import.meta.env.VITE_API_URL（docker compose 下 = "/api/dev"），
// vite proxy 會把 "/api/dev/..." 改寫成 "/api/v1/..." 後轉送 dashboard-be:8080；
// 所以這裡只寫 "/green/{resource}" 就好（不要再寫 /v1，會被 proxy double 加）。
const ENDPOINTS = {
	parks:      "/green/park",
	restaurant: "/green/restaurant",
	hotel:      "/green/hotel",
	recycle:    "/green/recycle",
	ubike:      "/green/ubike",
};

// geoJsonCache：map_config.index → FeatureCollection（覆寫成當前 city；不再 cache raw rows）
const geoJsonCache = new Map();

// per-component inflight：避免同一 component 連點 dropdown 時順序錯亂
const inflightByIndex = new Map(); // componentIndex → Promise

// 整體初次載入的 inflight（contentStore 進 kaobei dashboard 時用）
let bulkInflight = null;

// 用 raw axios（不走 src/router/axios.js 的 http），避免 401 強制登出 + 失敗時噴 notification 的 interceptor 副作用
async function fetchGreenResource(resourcePath, cityFilter) {
	const authStore = useAuthStore();
	const baseURL = import.meta.env.VITE_API_URL || "";
	const headers = authStore.token
		? { Authorization: `Bearer ${authStore.token}` }
		: {};
	return axios.get(`${baseURL}${resourcePath}`, {
		headers,
		params: { city: CITY_TO_BE_QUERY[cityFilter] },
		timeout: 10000,
	});
}

/* ───── chart_data 聚合邏輯 ───── */

function topN(map, n) {
	return [...map.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, n)
		.map(([x, y]) => ({ x, y }));
}

function normalizeTaipeiCity(city) {
	if (city === "台北市") return "臺北市";
	return city;
}

function extractDistrict(address) {
	const raw = typeof address === "string" ? address.trim() : "";
	if (!raw) return "";
	const match = raw.match(/(?:臺北市|台北市|新北市)([^0-9\s]{1,4}[區鎮市])/);
	return match?.[1] || "";
}

function getRestaurantDistrict(row) {
	return extractDistrict(row?.address) || normalizeTaipeiCity(row?.city) || "未分類";
}

// 公園 row 行政區/里欄位 schema 因縣市而異：
//   臺北市 row：pm_libie 有值（里名）、pm_type 通常空
//   新北市 row：pm_libie 為空、pm_type 是行政區（如「八里區」）
// fallback chain：pm_libie → pm_type → 從 pm_location address 抽
function getParkDistrict(row) {
	const libie = typeof row?.pm_libie === "string" ? row.pm_libie.trim() : "";
	if (libie) return libie;
	const type = typeof row?.pm_type === "string" ? row.pm_type.trim() : "";
	if (type) return type;
	return extractDistrict(row?.pm_location) || "";
}

// 把行政區字串尾字統一補「區」（BE 偶有「中正」/「中正區」混用）
function normalizeDistrict(s) {
	const v = typeof s === "string" ? s.trim() : "";
	if (!v) return "";
	if (v.endsWith("區") || v.endsWith("市") || v.endsWith("鎮") || v.endsWith("鄉")) return v;
	return `${v}區`;
}

// 從 hotel raw row 取行政區（優先用 town，退而求其次從 address regex 拆）
function getHotelTown(row) {
	const t = normalizeDistrict(row?.town);
	if (t) return t;
	const fromAddr = extractDistrict(row?.address);
	return fromAddr || "未分類";
}

const TRANSFORMS = {
	parks(rows) {
		// 跨雙北 row schema 用 fallback：臺北 pm_libie（里）/ 新北 pm_type（區）
		const counts = new Map();
		rows.forEach((row) => {
			const k = getParkDistrict(row);
			if (!k) return;
			counts.set(k, (counts.get(k) || 0) + 1);
		});
		// total 用 BE 回的 row 數量（含 pm_libie/pm_type 都沒有的未歸類筆數），
		// chart 上 top 10 仍只顯示能歸類的；wrapper 顯示 total_label 為「總合」
		return {
			series: [{ name: "公園數", data: topN(counts, 10) }],
			configPatch: { total_label: `${rows.length} 座` },
		};
	},
	restaurant(rows) {
		const counts = new Map();
		rows.forEach((row) => {
			const district = getRestaurantDistrict(row);
			counts.set(district, (counts.get(district) || 0) + 1);
		});
		return [{ name: "店家數", data: topN(counts, 8) }];
	},
	hotel(rows) {
		// ColumnChart + RadarChart 共用：依 town（行政區）分組計數，回 categories + 純數值陣列形式。
		// （RadarChart wrapper 必須要 categories 才畫得出來；ColumnChart 也支援同形式。）
		const counts = new Map();
		rows.forEach((row) => {
			const town = getHotelTown(row);
			counts.set(town, (counts.get(town) || 0) + 1);
		});
		// 雙北合計通常 ≤ 12 區，top 12 足以涵蓋；依數量遞減排序，視覺上長條從高→低
		const sorted = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12);
		return {
			series: [{ name: "家數", data: sorted.map(([, y]) => y) }],
			configPatch: { categories: sorted.map(([x]) => x) },
		};
	},
	recycle(rows) {
		// ColumnChart + 平均線：avg 取「全資料集每行政區家數平均」（非僅 top 10）
		const counts = new Map();
		rows.forEach((row) => {
			const d = normalizeDistrict(row?.district);
			if (!d) return;
			counts.set(d, (counts.get(d) || 0) + 1);
		});
		const all = [...counts.values()];
		const avg = all.length
			? Math.round((all.reduce((a, b) => a + b, 0) / all.length) * 10) / 10
			: 0;
		return {
			series: [{ name: "回收點數", data: topN(counts, 10) }],
			configPatch: avg > 0
				? { average_line: { value: avg, label: `雙北平均 ${avg}`, color: CHART_TOKENS.yellow } }
				: { average_line: null },
		};
	},
	ubike(rows) {
		// DonutChart / BarChart 共用：依 area（行政區）統計站數，top 8 + 「其他」彙總。
		// （BE 目前只回站點目錄、無 available_bikes/spots 即時欄位；改以站數分布展示。
		// 中央總和 = 雙北全部站數，包含未進 top 8 的小區。）
		const counts = new Map();
		rows.forEach((row) => {
			if (isInactiveUbike(row)) return;
			const area = typeof row.area === "string" ? row.area.trim() : "";
			if (!area) return;
			counts.set(area, (counts.get(area) || 0) + 1);
		});

		const TOP_N = 8;
		const sorted = [...counts.entries()].sort((a, b) => b[1] - a[1]);
		const data = sorted.slice(0, TOP_N).map(([x, y]) => ({ x, y }));
		const otherTotal = sorted.slice(TOP_N).reduce((sum, [, y]) => sum + y, 0);
		if (otherTotal > 0) data.push({ x: "其他", y: otherTotal });

		return [{ name: "Ubike", data }];
	},
};

/* ───── FeatureCollection 建構（給地圖圖層用） ───── */

function num(v) {
	const n = typeof v === "number" ? v : parseFloat(v);
	return Number.isFinite(n) ? n : null;
}

function isZeroCoordPair(lng, lat) {
	return lng === 0 && lat === 0;
}

function isInactiveUbike(row) {
	return row?.active === false || row?.active === 0 || row?.active === "0";
}

function fc(features) {
	return { type: "FeatureCollection", features };
}

function feature(coords, geomType, properties) {
	return {
		type: "Feature",
		geometry: { type: geomType, coordinates: coords },
		properties,
	};
}

const FC_BUILDERS = {
	parks(rows) {
		const features = [];
		for (const r of rows) {
			const lng = num(r.pm_Longitude);
			const lat = num(r.pm_Latitude);
			if (lng === null || lat === null) continue;
			features.push(feature([lng, lat], "Point", { ...r }));
		}
		return { park_greenspace: fc(features) };
	},
	restaurant(rows) {
		const features = [];
		for (const r of rows) {
			const lng = num(r.longitude);
			const lat = num(r.latitude);
			if (lng === null || lat === null) continue;
			features.push(feature([lng, lat], "Point", {
				...r,
				city: normalizeTaipeiCity(r.city),
				district: getRestaurantDistrict(r),
			}));
		}
		return { eco_restaurant: fc(features) };
	},
	hotel(rows) {
		const buckets = { gold: [], silver: [], other: [] };
		for (const r of rows) {
			const lng = num(r.longitude);
			const lat = num(r.latitude);
			if (lng === null || lat === null) continue;
			const note = r.note || "";
			let key = "other";
			if (note.includes("金級")) key = "gold";
			else if (note.includes("銀級")) key = "silver";
			buckets[key].push(feature([lng, lat], "Point", { ...r }));
		}
		return {
			eco_hotel_gold: fc(buckets.gold),
			eco_hotel_silver: fc(buckets.silver),
			eco_hotel_other: fc(buckets.other),
		};
	},
	recycle(rows) {
		const features = [];
		for (const r of rows) {
			const lng = num(r.longitude);
			const lat = num(r.latitude);
			if (lng === null || lat === null) continue;
			if (isZeroCoordPair(lng, lat)) continue;
			features.push(feature([lng, lat], "Point", { ...r }));
		}
		return { recycle_station: fc(features) };
	},
	ubike(rows) {
		const features = [];
		for (const r of rows) {
			if (isInactiveUbike(r)) continue;
			const lng = num(r.longitude);
			const lat = num(r.latitude);
			if (lng === null || lat === null) continue;
			features.push(feature([lng, lat], "Point", { ...r }));
		}
		return { youbike_availability: fc(features) };
	},
};

/* ───── ComponentConfig blueprints ───── */

function configBlueprints() {
	return [
		{
			id: 90001,
			index: "park_greenspace",
			city: CITY,
			name: "公園綠地",
			source: "Taipei Code Fest API",
			short_desc: "依里別排行 Top 10",
			long_desc: "顯示雙北地區公園、綠地、廣場之面積分布，格式為各行政區綠地面積矩形圖。資料來源為臺北市政府工務局公園路燈工程管理處公開資料，不定期更新，反映各區綠地資源配置現況。可作為城市綠地規劃與市民休閒選擇之參考依據。🟩 深綠色區塊：該行政區綠地面積較大，綠地資源充足。🟢 淺綠色區塊：該行政區綠地面積較小，綠地資源相對不足。",
			use_case: "臺北市與新北市的工務局及都市發展局可依據此可視化工具，快速掌握雙北地區公園綠地的面積分布與行政區差異。透過矩形圖直觀呈現各區綠地面積佔比，識別人均綠地低於 WHO 建議標準（9 平方公尺）的行政區，作為優先增設口袋公園或推動屋頂綠化政策的決策依據；同時，市民可透過地圖定位住家周邊的公園分布，選擇最近的綠地進行散步、運動或親子活動，減少開車前往遠處休閒場所的需求，以步行取代機動車輛，實踐日常低碳生活。",
			links: ["https://data.gov.tw/dataset/128366"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "park_greenspace",
			chart_config: {
				color: [CHART_TOKENS.green],
				colorRamp: GREEN_RAMP,
				types: ["TreemapChart", "BarChart"],
				unit: "座",
				categories: null,
				showDataLabels: true, // 切到橫向長條圖時數字顯示在條柱右側外（同餐廳）
			},
			chart_data: null,
			map_config: [
				{
					index: "park_greenspace",
					type: "circle",
					title: "公園綠地",
					paint: { "circle-color": CHART_TOKENS.green },
					property: [
						{ key: "pm_name", name: "名稱" },
						{ key: "pm_location", name: "地址" },
						{ key: "pm_libie", name: "里別" },
					],
					size: "small",
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "pm_libie", yParam: null } },
			history_config: null,
		},
		{
			id: 90002,
			index: "eco_restaurant",
			city: CITY,
			name: "環保餐廳",
			source: "Taipei Code Fest API",
			short_desc: "依行政區分布 Top 8",
			long_desc: "顯示雙北地區環保餐廳之分布數量，格式為各行政區環保餐廳數量橫向長條圖。資料來源為環境部環境即時通地圖公開資料，不定期更新，反映各區綠色餐飲資源之推廣成效與覆蓋密度。可作為推動環保標章餐廳認證與市民綠色消費選擇之參考依據。",
			use_case: "臺北市與新北市的環保局及觀光傳播局可依據此可視化工具，掌握雙北地區環保餐廳的分布密度與各行政區推廣成效。透過橫向長條圖比較各區環保餐廳數量，識別綠色餐飲資源不足的區域，作為推動環保標章餐廳認證輔導、擴大綠色消費網絡的政策參考；同時，市民外出用餐時可透過地圖快速搜尋住家或辦公室附近的環保餐廳，選擇減少一次性餐具、落實食材在地化與減少廚餘的餐飲場所，以日常飲食選擇支持永續經營的店家，讓每一餐都成為對環境友善的具體行動。",
			links: ["https://data.gov.tw/dataset/145036"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "eco_restaurant",
			chart_config: {
				color: [...UBIKE_AREA_PALETTE], // 跟 Ubike 同樣的黃→深黃漸層（distributed: true，每條一色）
				types: ["BarChart"],
				unit: "家",
				categories: null,
				showDataLabels: true, // 數字顯示在條柱右端外側
			},
			chart_data: null,
			map_config: [
				{
					index: "eco_restaurant",
					type: "circle",
					title: "環保餐廳",
					paint: { "circle-color": "#FB8C00" },
					property: [
						{ key: "name", name: "店名" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
					],
					size: "small",
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "district", yParam: null } },
			history_config: null,
		},
		{
			id: 90003,
			index: "eco_hotel",
			city: CITY,
			name: "環保旅宿",
			source: "Taipei Code Fest API",
			short_desc: "依環保標章分級",
			long_desc: "顯示雙北地區環保標章旅館之分布數量與等級結構，格式為各行政區環保旅宿數量縱向長條圖。資料來源為環境部環境即時通地圖公開資料，不定期更新，反映各區綠色觀光住宿資源之認證涵蓋率與等級分布。可作為推動旅宿業節能減碳輔導與旅客選擇低碳住宿之參考依據。圖示說明：長條圖高度代表該行政區環保旅宿總數，可搭配雷達圖模式檢視各區在不同等級之分布比較。",
			use_case: "臺北市與新北市的觀光傳播局及環保局可依據此可視化工具，快速掌握雙北地區環保標章旅宿的分布狀況與等級結構。透過縱向長條圖比較各行政區環保旅宿數量，洞察綠色觀光資源的集中度與缺口區域，作為推動旅宿業節能減碳輔導、擴大環保標章認證涵蓋率的施政依據；同時，國內外旅客在規劃雙北行程時，可透過地圖搜尋取得環保標章認證的住宿選擇，優先入住實施節水節電、減少一次性備品、落實廢棄物分類的旅館，以住宿選擇支持低碳觀光，讓每一次旅行都為城市永續盡一份力。",
			links: ["https://data.gov.tw/dataset/145035"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "eco_hotel",
			chart_config: {
				color: [CHART_TOKENS.purple],
				types: ["ColumnChart", "RadarChart"],
				unit: "家",
				categories: null,        // transform configPatch 在 fetch 後動態填入各 town
				showDataLabels: true,    // categories 形式預設關 dataLabels；顯式 opt-in 讓條柱頂端顯示家數
			},
			chart_data: null,
			map_config: [
				{
					index: "eco_hotel_gold",
					type: "circle",
					title: "金級",
					paint: { "circle-color": "#FFD700" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
				{
					index: "eco_hotel_silver",
					type: "circle",
					title: "銀級",
					paint: { "circle-color": "#C0C0C0" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
				{
					index: "eco_hotel_other",
					type: "circle",
					title: "其他",
					paint: { "circle-color": "#888888" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "town", yParam: null } },
			history_config: null,
		},
		{
			id: 90005,
			index: "recycle_station",
			city: CITY,
			name: "資源回收點",
			source: "Taipei Code Fest API",
			short_desc: "依行政區排行 Top 10",
			long_desc: "顯示雙北地區垃圾資源回收、廚餘回收限時收受點之分布數量，格式為各行政區回收站數量縱向長條圖，搭配雙北平均線標示。資料來源為臺北市政府環境保護局公開資料，不定期更新，反映各區回收據點配置密度。可作為優化回收站佈點與市民就近回收之參考依據。圖示說明：🟩 綠色長條（高於平均線）：該區回收站數量充足。🟡 黃色長條（低於平均線）：該區回收站數量不足，建議增設。",
			use_case: "臺北市與新北市的環保局及各區清潔隊可依據此可視化工具，快速掌握雙北地區社區資源回收站的設置密度與行政區差異。透過縱向長條圖搭配雙北平均線，一眼辨識回收站數量低於平均水準的區域，作為優先增設回收據點、調整垃圾車路線或推動定時定點回收政策的規劃依據；同時，市民可透過地圖定位住家最近的資源回收站，將日常產生的寶特瓶、廢紙、舊衣物就近送往回收，減少資源進入焚化爐的比例。當回收行為融入每日散步路線，順手做環保不再是額外負擔，而是生活中自然而然的永續習慣。",
			links: ["https://data.gov.tw/dataset/132357", "https://data.gov.tw/dataset/123351"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "recycle_station",
			chart_config: {
				color: [CHART_TOKENS.teal],
				types: ["ColumnChart"],
				unit: "點",
				categories: null,
				showDataLabels: true,
				average_line: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "recycle_station",
					type: "symbol",
					title: "資源回收點",
					paint: {},
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "open_time", name: "開放時間" },
					],
					size: null,
					icon: "triangle_green",
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "district", yParam: null } },
			history_config: null,
		},
		{
			id: 90006,
			index: "youbike_availability",
			city: CITY,
			name: "Ubike 站點",
			source: "Taipei Code Fest API",
			short_desc: "雙北站點目錄統計",
			long_desc: "顯示雙北地區 YouBike 2.0 站點目錄統計，內容包含總站數、臺北市站數、新北市站數與涵蓋行政區數。資料依目前後端提供之站點目錄資料計算，不包含即時可借可還數量。",
			use_case: "臺北市與新北市的交通局及相關單位可依據此可視化工具，快速掌握雙北 YouBike 站點分布規模與行政區覆蓋情形，作為站點佈建、服務涵蓋率與跨市站點配置評估的基礎參考；市民也可透過地圖查看站點位置與基本資訊。",
			links: ["https://data.gov.tw/dataset/137993", "https://data.gov.tw/dataset/146969"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "current",
			time_to: "current",
			update_freq: null,
			update_freq_unit: null,
			query_data: "youbike_availability",
			chart_config: {
				color: [...UBIKE_AREA_PALETTE],
				types: ["DonutChart", "BarChart"],
				unit: "站",
				categories: null,
				showDataLabels: true,
			},
			chart_data: null,
			map_config: [
				{
					index: "youbike_availability",
					type: "symbol",
					title: "Ubike 站點",
					paint: {},
					property: [
						{ key: "name", name: "站名" },
						{ key: "address", name: "地址" },
						{ key: "city", name: "縣市" },
						{ key: "area", name: "行政區" },
					],
					size: null,
					icon: "youbike",
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: null,
			history_config: null,
		},
	];
}

/* ───── public API ───── */

// 單一 resource 的 fetch + transform + FC build。永遠真打 BE，不讀 cache。
// 回傳 { ok, chartData, layerIndices }，layerIndices 是這次寫進 geoJsonCache 的 layer keys。
async function fetchAndTransformOne(resourceKey, cityFilter) {
	let rows;
	try {
		const res = await fetchGreenResource(ENDPOINTS[resourceKey], cityFilter);
		const data = res?.data?.data;
		rows = Array.isArray(data) ? data : [];
	} catch (err) {
		console.error(`[kaobei] ${resourceKey} API failed:`, err?.message || err);
		return { ok: false, chartData: null, layerIndices: [] };
	}

	// transform 可回 array（純 series）或 { series, configPatch }（需要動 chart_config 時）
	let chartData = null;
	let chartConfigPatch = null;
	try {
		const result = TRANSFORMS[resourceKey](rows);
		if (result && !Array.isArray(result) && Array.isArray(result.series)) {
			chartData = result.series;
			chartConfigPatch = result.configPatch || null;
		} else {
			chartData = result;
		}
	} catch (err) {
		console.error(`[kaobei] transform "${resourceKey}" failed`, err);
	}

	const layerIndices = [];
	try {
		const fcs = FC_BUILDERS[resourceKey](rows);
		for (const [layerIndex, featureCol] of Object.entries(fcs)) {
			geoJsonCache.set(layerIndex, featureCol);
			layerIndices.push(layerIndex);
		}
	} catch (err) {
		console.error(`[kaobei] FeatureCollection build "${resourceKey}" failed`, err);
	}

	return { ok: true, chartData, chartConfigPatch, layerIndices };
}

export const KAOBEI_LAYER_INDICES = new Set([
	"park_greenspace",
	"eco_restaurant",
	"eco_hotel_gold",
	"eco_hotel_silver",
	"eco_hotel_other",
	"recycle_station",
	"youbike_availability",
]);

export const KAOBEI_CONTRIBUTORS = {
	waiue0620: {
		id: null,
		user_id: "waiue0620",
		user_name: "waiue0620",
		link: "https://github.com/waiue0620",
		image: "https://avatars.githubusercontent.com/u/39615588?v=4",
		description: "",
		identity: "",
		include: true,
	},
	Mhanto0712: {
		id: null,
		user_id: "Mhanto0712",
		user_name: "Mhanto",
		link: "https://github.com/Mhanto0712",
		image: "https://avatars.githubusercontent.com/u/115198861?v=4",
		description: "",
		identity: "",
		include: true,
	},
	jadokao: {
		id: null,
		user_id: "jadokao",
		user_name: "Ming",
		link: "https://github.com/jadokao",
		image: "https://avatars.githubusercontent.com/u/35626307?v=4",
		description: "",
		identity: "",
		include: true,
	},
	Lydia584285: {
		id: null,
		user_id: "Lydia584285",
		user_name: "Lydia584285",
		link: "https://github.com/Lydia584285",
		image: "https://avatars.githubusercontent.com/u/79617259?v=4",
		description: "",
		identity: "",
		include: true,
	},
	mavisliu689: {
		id: null,
		user_id: "mavisliu689",
		user_name: "Mavis",
		link: "https://github.com/mavisliu689",
		image: "https://avatars.githubusercontent.com/u/16968359?v=4",
		description: "",
		identity: "",
		include: true,
	},
};

// 初次載入：對 6 個組件依各自 city 平行 fetch（contentStore 進 kaobei dashboard 時呼叫）
export async function loadKaobeiComponents() {
	if (bulkInflight) return bulkInflight;
	bulkInflight = (async () => {
		try {
			const configs = configBlueprints();
			const indexToConfig = new Map(configs.map((c) => [c.index, c]));
			await Promise.allSettled(
				Object.entries(INDEX_TO_RESOURCE).map(async ([compIndex, resourceKey]) => {
					const city = kaobeiCityFilters[compIndex];
					const result = await fetchAndTransformOne(resourceKey, city);
					const cfg = indexToConfig.get(compIndex);
					if (!cfg) return;
					cfg.chart_data = result.ok ? result.chartData : null;
					if (result.ok && result.chartConfigPatch) {
						Object.assign(cfg.chart_config, result.chartConfigPatch);
					}
				}),
			);
			return configs;
		} finally {
			bulkInflight = null;
		}
	})();
	return bulkInflight;
}

export function getKaobeiGeoJson(index) {
	return geoJsonCache.get(index);
}

// 切換單一組件 dropdown：set 該組件 city + 重打那一支 API + 更新 chart_data / map source
// view 端 @change-city listener 對 kaobei 組件呼叫此函式（每個組件獨立、不影響其他 5 個）
export async function setKaobeiCityFilter(componentIndex, value) {
	if (!INDEX_TO_RESOURCE[componentIndex]) return;          // 防呆：非 kaobei 組件
	if (!CITY_TO_BE_QUERY[value]) return;                     // 防呆：只接受 metrotaipei / taipei / newtaipei
	if (value === kaobeiCityFilters[componentIndex]) return;  // 同值不動
	kaobeiCityFilters[componentIndex] = value;
	await refetchOne(componentIndex);
}

// 重打單一組件對應的 BE → 重組 chart_data + geoJsonCache → 通知 contentStore / mapStore
async function refetchOne(componentIndex) {
	// 同 component 連點時等前一次完（保序），避免後 fetch 早回造成新 city 結果被舊 city 蓋掉
	const prev = inflightByIndex.get(componentIndex);
	if (prev) {
		try { await prev; } catch { /* swallow */ }
	}

	const reqCity = kaobeiCityFilters[componentIndex];
	const resourceKey = INDEX_TO_RESOURCE[componentIndex];

	const promise = fetchAndTransformOne(resourceKey, reqCity);
	inflightByIndex.set(componentIndex, promise);

	let result;
	try {
		result = await promise;
	} finally {
		if (inflightByIndex.get(componentIndex) === promise) {
			inflightByIndex.delete(componentIndex);
		}
	}

	// switch race 守門：await 完使用者已切回別的 city → 丟棄結果
	if (kaobeiCityFilters[componentIndex] !== reqCity) return;

	// Step A: in-place mutate component.chart_data。
	// cityDashboard.components 的 element 是 Pinia reactive proxy；改 .chart_data 會觸發
	// Pinia trap，DashboardComponent 接到 :config="item" prop 內部屬性變更，chart wrapper
	// 接 :series="config.chart_data" 也跟著 redraw。
	// currentDashboard.components 是 cityDashboard.components.filter() 出來的，element
	// reference 與 cityDashboard 共用，**只動 cityDashboard 一邊就好**（兩邊都動會在
	// scheduler flush race 中觸發 Vue internal "component is null" error）。
	const { useContentStore } = await import("../store/contentStore");
	const contentStore = useContentStore();
	const arr = contentStore.cityDashboard?.components;
	if (Array.isArray(arr) && result.ok && result.chartData !== null) {
		const target = arr.find((c) => c.index === componentIndex);
		if (target) {
			target.chart_data = result.chartData;
			if (result.chartConfigPatch) {
				// spread 換新 reference 確保 chart wrapper computed 重算（avg 變動時 annotation 跟著更新）
				target.chart_config = {
					...target.chart_config,
					...result.chartConfigPatch,
				};
			}
		}
	}

	// Step B: 通知 mapStore 只 reload 該 component 對應的 layer 子集（含清 byParam filter）
	if (result.ok) {
		const { useMapStore } = await import("../store/mapStore");
		const mapStore = useMapStore();
		mapStore.reloadKaobeiLayers(result.layerIndices);
	}
}

// Sidebar 列表用（contentStore.setDashboards 會 push 到 metrotaipei 群組）
export const KAOBEI_DASHBOARD_META = {
	index: "kaobei",
	name: "靠北儀表板",
	icon: "eco",
};
