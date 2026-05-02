// 黑客松「靠北儀表板」用：呼叫 6 支 BE API（同 envelope: { data, status, total }），
// 把 raw rows 轉成 ComponentConfig.chart_data + 各圖層的 FeatureCollection cache。
// contentStore 在 setCurrentDashboardAllContent 偵測 index="kaobei" 時直接呼叫 loadKaobeiComponents()；
// mapStore.fetchLocalGeoJson 對 kaobei_* prefix 的圖層呼叫 getKaobeiGeoJson(index) 從 cache 取。

import axios from "axios";
import { useAuthStore } from "../store/authStore";

const CITY = "metrotaipei";

// 端點：baseURL = import.meta.env.VITE_API_URL（docker compose 下 = "/api/dev"），
// vite proxy 會把 "/api/dev/..." 改寫成 "/api/v1/..." 後轉送 dashboard-be:8080；
// 所以這裡只寫 "/green/{resource}" 就好（不要再寫 /v1，會被 proxy double 加）。
const ENDPOINTS = {
	parks:      "/green/park",
	restaurant: "/green/restaurant",
	hotel:      "/green/hotel",
	walkpath:   "/green/walkpath",
	recycle:    "/green/recycle",
	ubike:      "/green/ublike", // BE 路徑 typo
};

// 全域 cache（module-scoped；route 切換時不清掉，下次切回來能立即顯示舊資料）
const apiCache = new Map();      // key (parks/restaurant/...) → raw rows[]
const geoJsonCache = new Map();  // map_config.index → FeatureCollection

// 同時間只准一個 batch fetch；併發呼叫 share 同個 Promise
let inflight = null;

// 用 raw axios（不走 src/router/axios.js 的 http），避免 401 強制登出 + 失敗時噴 notification 的 interceptor 副作用
async function fetchGreenResource(resourcePath) {
	const authStore = useAuthStore();
	const baseURL = import.meta.env.VITE_API_URL || "";
	const headers = authStore.token
		? { Authorization: `Bearer ${authStore.token}` }
		: {};
	return axios.get(`${baseURL}${resourcePath}`, {
		headers,
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

function countBy(rows, key) {
	const counts = new Map();
	for (const r of rows) {
		const k = r[key];
		if (!k) continue;
		counts.set(k, (counts.get(k) || 0) + 1);
	}
	return counts;
}

const TRANSFORMS = {
	parks(rows) {
		return [{ name: "公園數", data: topN(countBy(rows, "pm_libie"), 10) }];
	},
	restaurant(rows) {
		return [{ name: "店家數", data: topN(countBy(rows, "city"), 8) }];
	},
	hotel(rows) {
		const buckets = { 金級: 0, 銀級: 0, 其他: 0 };
		for (const r of rows) {
			const note = r.note || "";
			if (note.includes("金級")) buckets["金級"] += 1;
			else if (note.includes("銀級")) buckets["銀級"] += 1;
			else buckets["其他"] += 1;
		}
		return [
			{ name: "金級", type: "circle", value: buckets["金級"] },
			{ name: "銀級", type: "circle", value: buckets["銀級"] },
			{ name: "其他", type: "circle", value: buckets["其他"] },
		];
	},
	walkpath(rows) {
		const counts = countBy(rows, "grade");
		return [
			{
				name: "步道分級",
				data: [...counts.entries()].map(([x, y]) => ({ x, y })),
			},
		];
	},
	recycle(rows) {
		return [{ name: "回收點數", data: topN(countBy(rows, "district"), 10) }];
	},
	ubike(rows) {
		const totals = rows.reduce(
			(acc, r) => {
				if (r.active === false) return acc;
				acc.stations += 1;
				acc.bikes += Number(r.total_quantity) || 0;
				acc.available += Number(r.available_bikes) || 0;
				acc.spots += Number(r.available_spots) || 0;
				return acc;
			},
			{ stations: 0, bikes: 0, available: 0, spots: 0 },
		);
		return [
			{ name: "總站數", data: [totals.stations],   icon: "站" },
			{ name: "總車輛", data: [totals.bikes],      icon: "輛" },
			{ name: "可借",   data: [totals.available],  icon: "輛" },
			{ name: "可還",   data: [totals.spots],      icon: "位" },
		];
	},
};

/* ───── FeatureCollection 建構（給地圖圖層用） ───── */

function num(v) {
	const n = typeof v === "number" ? v : parseFloat(v);
	return Number.isFinite(n) ? n : null;
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
			features.push(feature([lng, lat], "Point", { ...r }));
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
	walkpath(rows) {
		const features = [];
		for (const r of rows) {
			const sLng = num(r.start_longitude);
			const sLat = num(r.start_latitude);
			const eLng = num(r.end_longitude);
			const eLat = num(r.end_latitude);
			if ([sLng, sLat, eLng, eLat].some((v) => v === null)) continue;
			features.push(
				feature([[sLng, sLat], [eLng, eLat]], "LineString", { ...r }),
			);
		}
		return { hiking_trail: fc(features) };
	},
	recycle(rows) {
		const features = [];
		for (const r of rows) {
			const lng = num(r.longitude);
			const lat = num(r.latitude);
			if (lng === null || lat === null) continue;
			features.push(feature([lng, lat], "Point", { ...r }));
		}
		return { recycle_station: fc(features) };
	},
	ubike(rows) {
		const features = [];
		for (const r of rows) {
			if (r.active === false) continue;
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
			links: ["https://data.gov.tw/dataset/128366", "https://data.gov.tw/dataset/124566"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "park_greenspace",
			chart_config: {
				color: ["#7CB342"],
				types: ["BarChart"],
				unit: "座",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "park_greenspace",
					type: "circle",
					title: "公園綠地",
					paint: { "circle-color": "#7CB342" },
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
			short_desc: "依縣市分布 Top 8",
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
				color: ["#FB8C00"],
				types: ["BarChart"],
				unit: "家",
				categories: null,
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
			map_filter: { mode: "byParam", byParam: { xParam: "city", yParam: null } },
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
				color: ["#FFD700", "#C0C0C0", "#888888"],
				types: ["MapLegend"],
				unit: "家",
				categories: null,
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
			map_filter: { mode: "byLayer", byParam: null },
			history_config: null,
		},
		{
			id: 90004,
			index: "hiking_trail",
			city: CITY,
			name: "親子步道",
			source: "Taipei Code Fest API",
			short_desc: "依步道分級分布",
			long_desc: "顯示雙北地區列管登山步道之空間分布與行政區密度，格式為行政區地圖（顏色深淺代表步道數量多寡）。資料來源為臺北市政府工務局公開資料，不定期更新。目前臺北市親山步道系統列管有 154 條步道，總長約 117 公里。可作為步道新闢規劃與市民親子戶外活動選擇之參考依據。圖示說明 🟩 深綠色區域：步道數量多，親山資源豐富。🟢 淺綠色區域：步道數量少，可考慮增設或串連既有路線。",
			use_case: "臺北市與新北市的觀光傳播局及工務局可依據此可視化工具，掌握雙北地區親子步道的空間分布與各行政區資源密度。透過行政區地圖以顏色深淺直觀呈現步道集中程度，識別步道資源豐富的區域（如北投、士林）與相對匱乏的區域，作為規劃新闢步道、改善既有步道親子友善設施（如無障礙坡道、休憩涼亭、飲水設施）的決策依據；同時，家長可透過地圖快速查找住家周邊適合親子同行的步道路線，利用週末假日以步行方式親近自然、增進親子互動，取代開車前往遠方景點的高碳排休閒模式，將戶外運動融入低碳生活日常。",
			links: ["https://data.gov.tw/dataset/145689"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "hiking_trail",
			chart_config: {
				color: ["#5a9cf8", "#42A5F5", "#1976D2", "#0D47A1"],
				types: ["DonutChart"],
				unit: "條",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "hiking_trail",
					type: "line",
					title: "親子步道",
					paint: { "line-color": "#5a9cf8" },
					property: [
						{ key: "route", name: "路線" },
						{ key: "district", name: "行政區" },
						{ key: "grade", name: "分級" },
						{ key: "total_length_m", name: "長度(m)" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "grade", yParam: null } },
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
				color: ["#4CAF50"],
				types: ["BarChart"],
				unit: "點",
				categories: null,
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
			short_desc: "全市即時可借可還統計",
			long_desc: "顯示雙北地區 YouBike 2.0 公共自行車的即時使用情況，格式為可借車輛數／全市車位數之圓餅圖。資料來源為臺北市政府交通局及新北市政府交通局公開資料，每 5-10 分鐘更新一次，反映即時的使用狀況與車輛調度情形。可作為交通監測與市民使用參考依據。圖示說明 🟡 黃色區段：2.0 在站車輛（可借）。⚡ 深黃色區段：2.0E 電輔車在站車輛。⬜ 灰色區段：空位（車輛已被借出使用中）。",
			use_case: "臺北市與新北市的交通局及環保局可依據此可視化工具，即時掌握雙北地區 YouBike 公共自行車的車輛使用狀況與站點供需平衡。透過圓餅圖呈現在站車輛與空位的即時比例，監控尖峰時段的車輛調度需求，作為優化車輛調配策略、評估新增站點選址的數據依據；同時，市民出門前可快速確認附近站點是否有車可借，選擇以 YouBike 取代機車或汽車完成短程移動。每一趟 YouBike 騎乘相較於機車通勤可減少約 0.12 公斤碳排放，當全市每日超過十萬人次使用 YouBike，累積的減碳效益等同於數千棵樹木全年的碳吸收量，使公共自行車成為城市邁向淨零排放最具規模的低碳運輸基礎建設。",
			links: ["https://data.gov.tw/dataset/137993", "https://data.gov.tw/dataset/146969"],
			contributors: ["waiue0620", "Mhanto0712", "jadokao", "Lydia584285", "mavisliu689"],
			time_from: "current",
			time_to: "current",
			update_freq: null,
			update_freq_unit: null,
			query_data: "youbike_availability",
			chart_config: {
				color: ["#5a9cf8", "#FFFFFF", "#888787"],
				types: ["TextUnitChart"],
				unit: null,
				categories: null,
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
						{ key: "available_bikes", name: "可借" },
						{ key: "available_spots", name: "可還" },
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

const FIXTURE_KEYS = ["parks", "restaurant", "hotel", "walkpath", "recycle", "ubike"];

/* ───── public API ───── */

async function runFetchAndTransform() {
	const configs = configBlueprints();
	const results = await Promise.allSettled(
		FIXTURE_KEYS.map((k) => fetchGreenResource(ENDPOINTS[k])),
	);
	results.forEach((res, idx) => {
		const key = FIXTURE_KEYS[idx];
		if (res.status === "rejected") {
			console.error(`[kaobei] ${key} API failed:`, res.reason?.message || res.reason);
			configs[idx].chart_data = null; // wrapper 顯示「組件資料異常」
			return;
		}
		const rows = res.value?.data?.data;
		const safeRows = Array.isArray(rows) ? rows : [];
		apiCache.set(key, safeRows);
		try {
			configs[idx].chart_data = TRANSFORMS[key](safeRows);
		} catch (err) {
			console.error(`[kaobei] transform "${key}" failed`, err);
			configs[idx].chart_data = null;
		}
		try {
			const fcs = FC_BUILDERS[key](safeRows);
			for (const [layerIndex, featureCol] of Object.entries(fcs)) {
				geoJsonCache.set(layerIndex, featureCol);
			}
		} catch (err) {
			console.error(`[kaobei] FeatureCollection build "${key}" failed`, err);
		}
	});
	return configs;
}

export const KAOBEI_LAYER_INDICES = new Set([
	"park_greenspace",
	"eco_restaurant",
	"eco_hotel_gold",
	"eco_hotel_silver",
	"eco_hotel_other",
	"hiking_trail",
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

export async function loadKaobeiComponents() {
	if (inflight) return inflight;
	inflight = (async () => {
		try {
			return await runFetchAndTransform();
		} finally {
			inflight = null;
		}
	})();
	return inflight;
}

export function getKaobeiGeoJson(index) {
	return geoJsonCache.get(index);
}

// Sidebar 列表用（contentStore.setDashboards 會 push 到 metrotaipei 群組）
export const KAOBEI_DASHBOARD_META = {
	index: "kaobei",
	name: "靠北儀表板",
	icon: "eco",
};
