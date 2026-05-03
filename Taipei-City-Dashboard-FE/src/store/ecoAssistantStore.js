/**
 * ecoAssistantStore — 減碳路徑規劃 AI 助手對話狀態
 *
 * 不持久化對話 (後端 ai_chatlog 已記錄)，僅維護 in-memory 當前 session。
 */
import { defineStore } from "pinia";

import { chatTwai } from "../assets/utilityFunctions/twaiClient.js";

import { useAuthStore } from "./authStore";

const API_BASE = import.meta.env.VITE_API_URL || "/api/dev";

/**
 * 已知 / 可信賴的雙北地名集合 (high-confidence 判定用)。
 * 加入規則: 名稱明確不會混淆 (有專名 prefix), 不能是「公園」「捷運站」這種泛稱。
 * 跟 BE placeDictionary 對齊概念, 不必精確同步 — 字典外的會降到 medium 由 LLM 處理。
 */
const KNOWN_PLACES = new Set([
	// FE Geocode_dictionary
	"台北市政府", "市府", "北市府",
	"新北市政府", "新北市府", "板橋市政府",
	// 常見地標
	"台北車站", "台北101", "象山", "象山公園",
	"中正紀念堂", "國父紀念館", "陽明山", "淡水", "板橋",
	"西門", "西門町", "東區", "信義商圈",
	"松山機場", "南港車站", "台北小巨蛋",
	// 雙北行政區 (作為「附近 / 區中心」使用是 OK 的, 但精確度比地標低)
	"信義區", "大安區", "中山區", "松山區", "萬華區", "中正區",
	"大同區", "北投區", "士林區", "內湖區", "南港區", "文山區",
	"板橋區", "新莊區", "三重區", "中和區", "永和區", "汐止區",
	"淡水區", "新店區",
]);

/**
 * 泛稱黑名單 — endpoint 等於這些字串就是模糊指代, 應該反問或交給 LLM 處理。
 * 規則: 沒有專名 prefix 的通用詞 (公園 ≠ 象山公園)。
 */
const AMBIGUOUS_TERMS = new Set([
	"公園", "捷運站", "車站", "夜市", "餐廳", "咖啡廳", "便利商店",
	"台北捷運站", "新北捷運站", "捷運", "MRT",
	"附近", "這裡", "那裡",
]);

function isKnownPlace(name) {
	if (!name) return false;
	return KNOWN_PLACES.has(name.trim());
}

function isAmbiguousTerm(name) {
	if (!name) return true;
	return AMBIGUOUS_TERMS.has(name.trim());
}

/**
 * 行程脈絡偵測 — 「兩天一夜 / 過夜 / 住宿」這類字眼意味著 user 不只要路線,
 * 還要旅館推薦; 「找個吃的 / 順路吃飯」要餐廳; 等等。
 * 這裡只回 boolean flags, 不直接 fetch — 由 send() 把 flags 加進 intent_hint
 * 引導 LLM 呼叫對應 tool (避免 LLM 忘記 system prompt 規則)。
 */
export function detectTripContext(text) {
	if (!text) return { wantsHotel: false, wantsMeal: false };
	return {
		wantsHotel: /兩天一夜|三天兩夜|過夜|住宿|找飯店|找旅館|住一晚|要訂房|找住的/.test(text),
		wantsMeal: /順路吃|找個吃|吃飯|午餐|晚餐|找餐廳|找咖啡/.test(text),
	};
}

/**
 * 路徑意圖信心分層:
 *   - high: regex 命中, 起終點都在 KNOWN_PLACES → 可直打 BE plan-route
 *   - medium: regex 命中但有一端不在字典或屬於泛稱 → 帶 hint 給 LLM, 不直打
 *   - low: regex 沒命中 → 完全交給 LLM tool calling
 *
 * 回傳形狀:
 *   { confidence: "high" | "medium", origin, destination }
 *   { confidence: "low" }
 */
export function classifyRouteIntent(text) {
	const intent = detectRouteIntent(text);
	if (!intent) return { confidence: "low" };
	const { origin, destination } = intent;
	const originOk = isKnownPlace(origin) && !isAmbiguousTerm(origin);
	const destOk = isKnownPlace(destination) && !isAmbiguousTerm(destination);
	if (originOk && destOk) {
		return { confidence: "high", origin, destination };
	}
	return { confidence: "medium", origin, destination };
}

/**
 * 旅遊時長 / 時段 / 客氣詞前綴 — 不影響地點抽取, 但會干擾 regex anchor, 需先 strip。
 * 規則: 只 strip 連續出現在「整句最前面」的, 中間出現不動 (避免誤刪地名)。
 */
const MODIFIER_PREFIXES = [
	"兩天一夜", "三天兩夜", "一日遊", "半日遊", "週末", "假日", "平日", "當天來回",
	"早上", "上午", "中午", "下午", "傍晚", "晚上", "今天", "明天", "後天",
	"我想", "我要", "我計畫", "我打算", "麻煩", "請", "幫我", "規劃", "幫忙",
];

function stripModifierPrefix(text) {
	let s = text;
	let changed = true;
	// 逐次剝, 因為可能多個 modifier 連著 (如「明天早上」「週末 麻煩」)
	while (changed) {
		changed = false;
		const trimmed = s.replace(/^[\s,，、]+/, "");
		for (const p of MODIFIER_PREFIXES) {
			if (trimmed.startsWith(p)) {
				s = trimmed.slice(p.length);
				changed = true;
				break;
			}
		}
		if (!changed) s = trimmed;
	}
	return s;
}

/**
 * 偵測「從 A 到 B」「A 走到 B」「A 至 B」之類的路徑規劃意圖。
 * 回 {origin, destination} 或 null。
 */
export function detectRouteIntent(text) {
	if (!text) return null;
	const normalized = stripModifierPrefix(text);
	const patterns = [
		/從\s*([^到走至→]{2,20}?)\s*(?:走到|到|至|→)\s*([^的，。！？\s]{2,20})/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*走到\s*([^的，。！？\s]{2,20})$/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*→\s*([^的，。！？\s]{2,20})$/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*至\s*([^的，。！？\s]{2,20})$/,
		// 「X到Y」沒有「從」，較寬鬆 — 整句必須是 X+到+Y 沒其他句綴
		/^([^到走至→\s的，。！？]{2,20}?)\s*到\s*([^的，。！？\s]{2,20})$/,
	];
	for (const re of patterns) {
		const m = normalized.match(re);
		if (m) return { origin: m[1].trim(), destination: m[2].trim() };
	}
	return null;
}

/**
 * 雙北行政區 + 常見地標 中心座標 (供 POI 意圖偵測, 與 BE geocode 字典對齊大略值)
 * 不需要太精確 — 後端 find-pois 會以這個為圓心 + radius_km 過濾。
 * 地標排在最前面以便較具體的字串先 match
 * (例如 chip 「台北車站附近的公園」, 走 FE 直查避免依賴 LLM 加 META 標記)。
 */
const DISTRICT_COORDS = {
	// 常見地標 (與 chip / BE placeDictionary 對齊)
	台北車站: { lat: 25.0478, lng: 121.5170 },
	"台北101": { lat: 25.0339, lng: 121.5645 },
	台北市政府: { lat: 25.0376, lng: 121.5644 },
	新北市政府: { lat: 25.0124, lng: 121.4664 },
	板橋車站: { lat: 25.0143, lng: 121.4636 },
	大安森林公園: { lat: 25.0297, lng: 121.5354 },
	象山: { lat: 25.0269, lng: 121.5710 },
	陽明山: { lat: 25.1554, lng: 121.5615 },
	淡水: { lat: 25.1700, lng: 121.4400 },
	// 台北 12 區
	信義區: { lat: 25.033, lng: 121.564 },
	大安區: { lat: 25.026, lng: 121.543 },
	中山區: { lat: 25.064, lng: 121.531 },
	松山區: { lat: 25.058, lng: 121.563 },
	萬華區: { lat: 25.037, lng: 121.500 },
	中正區: { lat: 25.032, lng: 121.519 },
	大同區: { lat: 25.066, lng: 121.515 },
	北投區: { lat: 25.132, lng: 121.499 },
	士林區: { lat: 25.094, lng: 121.526 },
	內湖區: { lat: 25.083, lng: 121.589 },
	南港區: { lat: 25.054, lng: 121.607 },
	文山區: { lat: 24.989, lng: 121.570 },
	// 新北常用
	板橋區: { lat: 25.013, lng: 121.466 },
	新莊區: { lat: 25.036, lng: 121.450 },
	三重區: { lat: 25.061, lng: 121.486 },
	中和區: { lat: 25.000, lng: 121.499 },
	永和區: { lat: 25.007, lng: 121.515 },
	汐止區: { lat: 25.062, lng: 121.642 },
	淡水區: { lat: 25.169, lng: 121.444 },
	新店區: { lat: 24.967, lng: 121.541 },
};

/** POI 類別關鍵字 → BE category 名 */
const POI_CATEGORY_KEYWORDS = {
	park: ["公園", "綠地"],
	restaurant: ["餐廳", "餐館", "環保餐廳", "咖啡廳", "咖啡店", "咖啡館"],
	hotel: ["旅館", "飯店", "民宿", "環保旅館"],
	recycle: ["回收站", "回收點", "資源回收"],
	ubike: ["YouBike", "youbike", "Ubike", "ubike", "UBike", "微笑單車", "共享單車", "公共自行車", "單車站", "腳踏車站", "U-bike"],
};

/**
 * 偵測 POI 查詢意圖 (區/地標 + 類別)。回 {center, radiusKm, categories, districts} 或 null。
 *
 * 範例:
 *   「信義區的環保餐廳」      → { center: 信義區, categories: [restaurant] }
 *   「大安區附近的回收站」    → { center: 大安區, categories: [recycle] }
 *   「台北車站附近的公園」    → { center: 台北車站, categories: [park] }
 *   「從台北車站到台北101」   → null (route 句型, 讓 plan_eco_route 處理)
 */
export function detectPOIIntent(text) {
	if (!text) return null;
	// route 句型 (從 X 到 Y / X→Y / X 至 Y) 一律讓給 plan_eco_route, 不要重複觸發 POI 搜尋
	if (/從.+(到|至|→|走到)|→|至/.test(text)) return null;
	let matchedDistrict = null;
	for (const d of Object.keys(DISTRICT_COORDS)) {
		if (text.includes(d) || text.includes(d.replace("區", ""))) {
			matchedDistrict = d;
			break;
		}
	}
	if (!matchedDistrict) return null;

	const matchedCats = [];
	for (const [cat, keywords] of Object.entries(POI_CATEGORY_KEYWORDS)) {
		if (keywords.some((k) => text.includes(k))) matchedCats.push(cat);
	}
	if (!matchedCats.length) return null;

	return {
		center: DISTRICT_COORDS[matchedDistrict],
		radiusKm: 1.5,
		categories: matchedCats,
		districts: [matchedDistrict],
	};
}

/**
 * 從 LLM 回覆抽出最後一個 [META:...] 標記。
 * LLM 不一定會把 META 放單獨一行 (例如直接黏在最後一句後面),
 * 所以全文掃, 不要求換行錨定。
 */
export function parseMetaMarker(text) {
	if (!text) return null;
	let last = null;
	const re = /\[META:([^\]]+)\]/g;
	for (const match of text.matchAll(re)) last = match;
	if (!last) return null;
	const parts = last[1].split("|");
	const intent = parts[0].trim();
	const params = {};
	for (const p of parts.slice(1)) {
		const eq = p.indexOf("=");
		if (eq < 0) continue;
		params[p.slice(0, eq).trim()] = p.slice(eq + 1).trim();
	}
	return { intent, params, raw: last[0] };
}

/** 各交通方式排放因子 (g CO2e / 人公里), 與 BE eco/carbon.go 同步 */
const EMISSION = { car: 192, scooter: 78, bus: 49, mrt: 33, youbike: 0, walk: 0 };
const TREE_GRAMS_PER_YEAR = 21770;

// 餐飲: 一般葷食 2,500 g − 環保餐廳 1,800 g = 700 g/餐 (-28%, 節能 + 在地食材)
// 住宿: 一般旅店 29,000 g − Green Coin 環保旅店 23,000 g = 6,000 g/晚 (-20%)
const DINING_BASELINE_G = 2500;
const DINING_ECO_G = 1800;
const LODGING_BASELINE_G = 29000;
const LODGING_ECO_G = 23000;
const DINING_SAVING_PER_MEAL_G = DINING_BASELINE_G - DINING_ECO_G;
const LODGING_SAVING_PER_NIGHT_G = LODGING_BASELINE_G - LODGING_ECO_G;

function clampNonNegInt(n) {
	const v = Math.floor(Number(n));
	return Number.isFinite(v) && v > 0 ? v : 0;
}

export function calcDiningSavingG(meals) {
	return DINING_SAVING_PER_MEAL_G * clampNonNegInt(meals);
}

export function calcLodgingSavingG(nights) {
	return LODGING_SAVING_PER_NIGHT_G * clampNonNegInt(nights);
}

/** 對標單一交通方式算減碳 (取首條路線距離) */
export function calcTransportSavingG(route, baselineKey = "car") {
	if (!route?.routes?.length) return 0;
	const factor = EMISSION[baselineKey] ?? EMISSION.car;
	const km = route.routes[0].distance_m / 1000;
	return Math.max(0, (factor - EMISSION.walk) * km);
}

/**
 * 一日總減碳摘要: 結果區 + 總和。
 * meals/nights 為 0 時不顯示對應段落, 便於單獨估算交通。
 */
export function buildTotalCarbonSummary({ route, baselineKey = "car", meals = 0, nights = 0 } = {}) {
	if (!route?.routes?.length) return "";
	const m = clampNonNegInt(meals);
	const n = clampNonNegInt(nights);
	const transportG = calcTransportSavingG(route, baselineKey);
	const diningG = calcDiningSavingG(m);
	const lodgingG = calcLodgingSavingG(n);
	const totalG = transportG + diningG + lodgingG;

	const lines = [`從${route.start_name}到${route.end_name} — 一日減碳估算\n`];
	lines.push("📊 結果");
	lines.push(`・交通：${(transportG / 1000).toFixed(2)} kg CO₂e`);
	if (m > 0) lines.push(`・餐飲：${(diningG / 1000).toFixed(2)} kg CO₂e`);
	if (n > 0) lines.push(`・住宿：${(lodgingG / 1000).toFixed(2)} kg CO₂e`);
	lines.push("");
	const trees = totalG / TREE_GRAMS_PER_YEAR;
	lines.push(`🌳 總減碳：${(totalG / 1000).toFixed(2)} kg CO₂e ≈ ${trees.toFixed(3)} 棵樹一年固碳`);
	return lines.join("\n");
}

/** 對標多種交通方式的減碳基準 */
const BASELINES = [
	{ key: "car", label: "汽車", factor: EMISSION.car },
	{ key: "scooter", label: "機車", factor: EMISSION.scooter },
	{ key: "bus", label: "公車", factor: EMISSION.bus },
	{ key: "mrt", label: "捷運", factor: EMISSION.mrt },
];

/** FE 端直接算減碳量摘要, 對標 4 種交通方式 (避免讓 LLM 跑 calc_carbon_saving 工具迴圈卡住) */
export function buildCarbonSavingReply(result) {
	if (!result?.routes?.length) return "";
	const lines = [`從${result.start_name}到${result.end_name}走路相比其他交通方式的減碳量：\n`];
	for (const r of result.routes) {
		const km = r.distance_m / 1000;
		lines.push(`📍 ${r.label} (${km.toFixed(1)} km)`);
		for (const b of BASELINES) {
			const savedG = (b.factor - EMISSION.walk) * km;
			if (savedG <= 0) continue;
			const savedKg = savedG / 1000;
			const trees = savedG / TREE_GRAMS_PER_YEAR;
			lines.push(
				`  vs ${b.label} (${b.factor} g/km)：減碳 ${savedKg.toFixed(2)} kg CO₂e ≈ ${trees.toFixed(3)} 棵樹一年固碳`
			);
		}
		lines.push("");
	}
	lines.push("💡 排放因子來源：環境部、北捷年報；樹木年固碳 21.77 kg/棵（林業署）。");
	lines.push("選擇你「原本會用」的交通方式對照即可。");
	return lines.join("\n");
}

/** 結構化 payload 給 EcoCarbonCard 用; 不含 UI 字串拼接, 純資料 */
export function buildCarbonSavingPayload(result) {
	if (!result?.routes?.length) return null;
	const routes = result.routes.map((r) => {
		const km = r.distance_m / 1000;
		const baselines = BASELINES.map((b) => {
			const savedG = Math.max(0, (b.factor - EMISSION.walk) * km);
			return {
				key: b.key,
				label: b.label,
				factor: b.factor,
				savedKg: savedG / 1000,
				trees: savedG / TREE_GRAMS_PER_YEAR,
			};
		}).filter((x) => x.savedKg > 0);
		return {
			id: r.route_id,
			label: r.label,
			km,
			baselines,
			hero: baselines[0] || null, // 預設首位 (汽車) 為對照
		};
	});
	return {
		startName: result.start_name,
		endName: result.end_name,
		routes,
	};
}

/** 一行頭條摘要; 顯示在訊息泡泡內當卡片的 caption */
export function buildCarbonSavingHeadline(result) {
	const payload = buildCarbonSavingPayload(result);
	if (!payload?.routes?.length) return "";
	const top = payload.routes[0];
	if (!top?.hero) return `從${payload.startName}到${payload.endName}的減碳量`;
	return `從${payload.startName}到${payload.endName}，最多省 ${top.hero.savedKg.toFixed(2)} kg CO₂e（vs ${top.hero.label}）`;
}

/** 從 PlanResult 組一段純文字摘要 (LLM fallback / 快速回覆用) */
export function buildFallbackSummary(result) {
	if (!result?.routes?.length) return "";
	const lines = [`從${result.start_name}到${result.end_name}的減碳路線：`];
	for (const r of result.routes) {
		const km = (r.distance_m / 1000).toFixed(1);
		const min = r.estimated_minutes;
		const pts = r.green_points_passed?.length || 0;
		lines.push(`・${r.label}：${km} km · ${min} 分 · 綠點 ${pts}`);
	}
	return lines.join("\n");
}

/** 把所有 [META:...] 區塊從顯示文字中拿掉 (不論是不是獨立一行) */
export function stripMetaLine(text) {
	if (!text) return text;
	return text
		.replace(/\[META:[^\]]+\]/g, "")
		.split(/\r?\n/)
		.map((l) => l.replace(/[ \t]+$/, ""))
		.filter((l, i, arr) => !(l === "" && (i === 0 || arr[i - 1] === "")))
		.join("\n")
		.trim();
}

// 判斷字串是否為「目前位置」之類的代名詞 (FE 端攔截, 用瀏覽器 GPS 解析)
const CURRENT_LOC_KEYWORDS = [
	"目前位置",
	"現在位置",
	"當前位置",
	"我的位置",
	"我現在的地方",
	"我這裡",
	"我所在",
];
export function isCurrentLocationKeyword(name) {
	if (!name) return false;
	return CURRENT_LOC_KEYWORDS.some((k) => name.includes(k));
}

/** 用瀏覽器 navigator.geolocation 取得目前座標 */
export function getCurrentPosition() {
	return new Promise((resolve, reject) => {
		if (!navigator.geolocation) {
			reject(new Error("瀏覽器不支援定位"));
			return;
		}
		navigator.geolocation.getCurrentPosition(
			(pos) => resolve({ lat: pos.coords.latitude, lng: pos.coords.longitude }),
			(err) => reject(new Error(err.message || "定位被拒絕，請點地圖選起點")),
			{ enableHighAccuracy: false, timeout: 7000, maximumAge: 60000 }
		);
	});
}

async function fetchFindPOIs({ lat, lng, radiusKm, categories, districts, token }) {
	const body = {
		center: { Lat: lat, Lng: lng },
		radius_km: radiusKm,
		categories,
		limit: 20,
	};
	if (districts?.length) body.districts = districts;
	const res = await fetch(`${API_BASE}/ai/eco/find-pois`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${token}`,
		},
		body: JSON.stringify(body),
	});
	if (!res.ok) throw new Error(`find-pois HTTP ${res.status}`);
	const json = await res.json();
	return json?.data || [];
}

async function fetchPlanRoute({ origin, destination, originCoord, destinationCoord, token }) {
	const body = {};
	if (origin) body.origin = origin;
	if (destination) body.destination = destination;
	if (originCoord) body.origin_coord = originCoord;
	if (destinationCoord) body.destination_coord = destinationCoord;
	const res = await fetch(`${API_BASE}/ai/eco/plan-route`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: `Bearer ${token}`,
		},
		body: JSON.stringify(body),
	});
	if (!res.ok) throw new Error(`plan-route HTTP ${res.status}`);
	const json = await res.json();
	return json?.data || null;
}

export const useEcoAssistantStore = defineStore("ecoAssistant", {
	state: () => ({
		open: false,                 // 面板是否開啟
		messages: [],                // [{role, content}]
		isStreaming: false,
		session: null,               // TWCC session id
		currentRoute: null,          // 解析自工具回傳的 PlanResult
		errorMessage: "",
		_currentStream: null,        // {cancel, promise}
		// 地圖點選 / 拖曳起終點
		pickMode: "idle",            // 'idle' | 'origin' | 'destination'
		manualOrigin: null,          // {lat, lng} | null
		manualDest: null,            // {lat, lng} | null
		// 圖層顯示開關 (供 RouteCard UI 與 EcoMapOverlay 共享)
		layerVisibility: { shortest: true, balanced: true, greenest: true, pois: true, search: true },
		// 從 find_eco_pois 拿到的 POI 列表 (對話「附近 / 沿途有什麼 X」場景)
		currentSearchPOIs: null, // { categories, center, radius, items: [...] } | null
	}),
	actions: {
		togglePanel() {
			this.open = !this.open;
		},
		closePanel() {
			this.open = false;
		},
		reset() {
			this.cancelStream();
			this.messages = [];
			this.session = null;
			this.currentRoute = null;
			this.errorMessage = "";
			// 清除地圖上殘留的 search POI marker 與手動起終點
			// 否則 refresh 鈕只會清掉文字, 地圖一片綠樹擦不掉
			this.currentSearchPOIs = null;
			this.manualOrigin = null;
			this.manualDest = null;
			this.pickMode = "idle";
		},
		cancelStream() {
			if (this._currentStream) {
				try { this._currentStream.cancel(); } catch {}
				this._currentStream = null;
			}
			this.isStreaming = false;
		},
		async send(text) {
			if (!text?.trim()) return;
			// 若上一筆仍在串流: 取消, 並把那一輪未完成的 user+empty assistant
			// 一起清掉, 避免出現兩筆連續 user 訊息 (會被 TWCC 拒絕)
			if (this.isStreaming) {
				this.cancelStream();
				while (this.messages.length > 0) {
					const last = this.messages[this.messages.length - 1];
					if (last.role === "assistant" && !last.content) {
						this.messages.pop();
					} else if (last.role === "user") {
						this.messages.pop();
						break;
					} else {
						break;
					}
				}
			} else {
				// 即使 isStreaming=false 也可能殘留空 assistant (例: 上一輪 onError
				// 後沒清乾淨), 留著會讓下一輪變成 [user, empty-asst, user, ...]
				// 兩筆 user 連在一起被 TWCC 拒絕, 對話直接卡死。
				while (this.messages.length > 0) {
					const last = this.messages[this.messages.length - 1];
					if (last.role === "assistant" && !last.content) {
						this.messages.pop();
					} else {
						break;
					}
				}
			}
			// 若使用者重複送同樣內容 (例: 連點 chip), 不再追加新 user 訊息
			const prev = this.messages[this.messages.length - 1];
			if (prev?.role === "user" && prev.content === text) return;

			this.errorMessage = "";
			this.currentSearchPOIs = null;
			// userMsg.content 是顯示給使用者看的乾淨原文; apiContent (若有) 是
			// 實際送給 LLM 的版本, 可能附帶 <intent_hint> 引導 tool calling
			const userMsg = { role: "user", content: text };
			this.messages.push(userMsg);
			// 預先放一個空白 assistant 訊息，串流時逐 chunk 累加
			const assistantIndex = this.messages.length;
			this.messages.push({ role: "assistant", content: "" });

			this.isStreaming = true;

			const auth = useAuthStore();
			const token = auth?.token || localStorage.getItem("token") || "";
			if (!token) {
				this.errorMessage = "請先登入再使用小碳寶";
				this.isStreaming = false;
				return;
			}

			// 路徑意圖信心分層:
			//   high   → 兩端皆已知地名, 直打 BE plan-route 省 LLM 一輪 tool call 延遲
			//   medium → regex 命中但有泛稱 (例「台北捷運站」), 不直打;
			//            送給 LLM 的 user content 附 <intent_hint>, 引導 LLM
			//            呼叫 plan_eco_route 或反問, 而不是憑空亂回
			//   low    → 完全交給 LLM tool calling, FE 不做任何 NLP
			const cls = classifyRouteIntent(text);
			const trip = detectTripContext(text);
			const tripCats = [];
			if (trip.wantsHotel) tripCats.push("hotel");
			if (trip.wantsMeal) tripCats.push("restaurant");
			const tripHint = tripCats.length
				? `\n額外提示: 使用者語意需要 ${tripCats.join(", ")} 推薦, 規劃路線後務必再呼叫 find_eco_pois categories=[${tripCats.map((c) => `"${c}"`).join(",")}] 補上, 不要漏掉。`
				: "";

			if (cls.confidence === "high") {
				this._planFromIntent({ origin: cls.origin, destination: cls.destination }, token)
					.catch((e) => console.warn("plan-route fetch failed:", e.message));
				// high 也可能漏 trip context (LLM 拿到結構化結果就停了), 仍補 hint 提醒
				if (tripHint) {
					userMsg.apiContent = `${text}\n\n<intent_hint>${tripHint.trim()}</intent_hint>`;
				}
			} else if (cls.confidence === "medium") {
				userMsg.apiContent =
					`${text}\n\n<intent_hint>看似 origin="${cls.origin}", destination="${cls.destination}", `
					+ `但有一端不在已知地名清單。請呼叫 plan_eco_route 工具或反問使用者澄清, `
					+ `不要憑空生成路線距離 / 分鐘 / 經過點。${tripHint}</intent_hint>`;
			} else if (tripHint) {
				// low 也可能有 trip context (例「找環保旅館」沒 A→B 結構)
				userMsg.apiContent = `${text}\n\n<intent_hint>${tripHint.trim()}</intent_hint>`;
			}

			// POI 意圖也直打 BE — 不依賴 LLM 加 META 標記, 確保地圖一定有 marker
			// LLM 對 POI 查詢常幻覺「沒有」, 用這個 promise 在 onDone 覆蓋 LLM 文字成真實清單
			const poiIntent = detectPOIIntent(text);
			let poiFetchPromise = null;
			if (poiIntent) {
				poiFetchPromise = fetchFindPOIs({
					lat: poiIntent.center.lat,
					lng: poiIntent.center.lng,
					radiusKm: poiIntent.radiusKm,
					categories: poiIntent.categories,
					districts: poiIntent.districts,
					token,
				})
					.then((items) => {
						const withCoord = (items || []).filter((p) => Number.isFinite(p?.lat) && Number.isFinite(p?.lng));
						console.info("[eco] FE detectPOIIntent fetch:", {
							intent: poiIntent,
							total: items?.length || 0,
							withCoord: withCoord.length,
						});
						this.currentSearchPOIs = {
							categories: poiIntent.categories,
							districts: poiIntent.districts,
							center: poiIntent.center,
							radius: poiIntent.radiusKm,
							items: items || [],
						};
						return { items: items || [], withCoord, intent: poiIntent };
					})
					.catch((e) => {
						console.warn("find-pois fetch failed:", e.message);
						return null;
					});
			}

			// BE 端會自動注入 system prompt + tools, FE 只送 user/assistant 歷史
			// 為避免 TWCC 16k context 爆掉, 只送最後 6 輪 (12 則訊息)
			// 太舊的對話內容對當下查詢價值不大
			// TWCC 16k token 上限; assistant 訊息可能 verbose (列幾家餐廳就 ~600 tokens),
			// 6 則 = 3 輪 user/assistant 來回, 對話脈絡夠用又不會把 context 撐爆
			const HISTORY_CAP = 6;
			const all = this.messages.slice(0, -1);
			const trimmed = all.length > HISTORY_CAP ? all.slice(-HISTORY_CAP) : all;
			// medium-confidence 的 user 訊息會帶 apiContent (含 <intent_hint>), 用它送給 LLM;
			// 顯示用的 content 保持原文乾淨
			const apiMessages = trimmed.map((m) => ({ role: m.role, content: m.apiContent || m.content }));

			// 60 秒沒收到任何字 / done 事件就 abort, 避免 typing dots 卡死
			let timeoutId = null;
			const STREAM_TIMEOUT_MS = 60000;
			const armTimeout = () => {
				if (timeoutId) clearTimeout(timeoutId);
				timeoutId = setTimeout(() => {
					if (this._currentStream) {
						this._currentStream.cancel();
						this.errorMessage = "AI 回應超時 (60 秒)，請再試一次";
						this.isStreaming = false;
						this._currentStream = null;
						const last = this.messages[this.messages.length - 1];
						if (last && last.role === "assistant" && !last.content) {
							this.messages.pop();
						}
					}
				}, STREAM_TIMEOUT_MS);
			};
			armTimeout();

			this._currentStream = chatTwai({
				messages: apiMessages,
				session: this.session,
				token,
				onChunk: (chunk) => {
					this.messages[assistantIndex].content += chunk;
					armTimeout(); // 每收到一個 chunk 就重設計時, 避免長路線回覆中途被砍
				},
				onDone: async (full) => {
					if (timeoutId) clearTimeout(timeoutId);
					this.isStreaming = false;
					this._currentStream = null;
					const last = this.messages[this.messages.length - 1];
					// LLM 文字若為空, 先補 routeReady fallback; 不立即 pop, 讓下方 poiFetchPromise / 其他 fallback 有機會接管
					if (last && last.role === "assistant" && !last.content) {
						if (last.routeReady && this.currentRoute) {
							last.content = buildFallbackSummary(this.currentRoute);
						}
					}
					// 不論 parseMetaMarker 是否成功, 都把 [META:...] 行從顯示內容剝掉,
					// 避免 LLM 偶發產生稍微不符 regex 的 META 漏出來給使用者看。
					if (last && last.role === "assistant" && last.content) {
						last.content = stripMetaLine(last.content);
					}
					// 抓 LLM META 標記 -> 觸發對應 BE fetch
					const meta = parseMetaMarker(full);
					await this._handleMeta(meta, token);
					// 後備: 仍試一次 regex extract
					if (!this.currentRoute) this._tryExtractRoute(full);
					// FE detectPOIIntent 拿到的真實清單 — 只在 LLM 沒寫文字 (空內容 / 太短) 時才覆蓋。
					// 之前是無條件覆蓋, 結果把 LLM 的個性版回覆都吃掉, 體驗變冷冰冰 bullet list。
					// 規則: LLM 內容 < 30 字 (純列表 fallback 才會這麼短) 才用 FE 清單; 否則信任 LLM 文字。
					if (poiFetchPromise) {
						try {
							const r = await poiFetchPromise;
							const llmText = (last?.content || "").trim();
							const llmLooksEmpty = llmText.length < 30;
							if (r && r.withCoord?.length && last && last.role === "assistant" && llmLooksEmpty) {
								const labelMap = { park: "公園", restaurant: "環保餐廳", hotel: "環保旅館", recycle: "回收站", ubike: "YouBike 站點" };
								const labels = r.intent.categories.map((c) => labelMap[c] || c).join("・");
								const districtTag = r.intent.districts?.[0] || "附近";
								const lines = [`📍 ${districtTag}的${labels}：`];
								r.withCoord.slice(0, 5).forEach((p) => {
									lines.push(`・${p.name || "(未命名)"}`);
								});
								if (r.withCoord.length > 5) {
									lines.push(`(還有 ${r.withCoord.length - 5} 筆, 想看更多再跟我說)`);
								}
								last.content = lines.join("\n");
							}
						} catch (_) {
							// 已在 .catch 內 console.warn 過, 此處忽略
						}
					}
					// 最後保險: 若所有 fallback 都接不到, 給一句友善提示, 別把使用者晾在那
					if (last && last.role === "assistant" && !last.content) {
						last.content = "嗯, 這個我這次沒抓到資料 😅 換個問法或重新查詢看看？";
					}
				},
				onError: (err) => {
					if (timeoutId) clearTimeout(timeoutId);
					this.errorMessage = err.message || "AI 助手暫時無回應，請稍後再試";
					this.isStreaming = false;
					this._currentStream = null;
					// 把那一輪殘留的空 assistant 清掉, 避免下一輪出現兩筆連續 user
					const last = this.messages[this.messages.length - 1];
					if (last && last.role === "assistant" && !last.content) {
						this.messages.pop();
					}
				},
			});

			try {
				await this._currentStream.promise;
			} catch (e) {
				if (!String(e?.message).includes("aborted")) {
					this.errorMessage = e.message;
				}
				this.isStreaming = false;
			}
		},
		// 一日總減碳 (交通 + 餐飲 + 住宿) — FE 端直接算, 不打 LLM
		injectTotalCarbonReply(text, { baselineKey = "car", meals = 2, nights = 1 } = {}) {
			if (!this.currentRoute) {
				this.errorMessage = "先規劃一條路線, 再估算總減碳";
				return false;
			}
			if (this.isStreaming) {
				this.cancelStream();
				while (this.messages.length > 0) {
					const last = this.messages[this.messages.length - 1];
					if (last.role === "assistant" && !last.content) this.messages.pop();
					else if (last.role === "user") { this.messages.pop(); break; }
					else break;
				}
			}
			this.messages.push({ role: "user", content: text });
			this.messages.push({
				role: "assistant",
				content: buildTotalCarbonSummary({
					route: this.currentRoute,
					baselineKey,
					meals,
					nights,
				}),
			});
			return true;
		},
		// FE 端直接算減碳量, 不打 LLM (避免 tool loop 卡住)
		// 把 user 訊息與計算結果直接 push 進 messages
		injectCarbonReply(text) {
			if (!this.currentRoute) {
				this.errorMessage = "先規劃一條路線, 再問減碳量";
				return false;
			}
			// 與一般 send 流程一致: 維持 user/assistant 交替
			if (this.isStreaming) {
				this.cancelStream();
				while (this.messages.length > 0) {
					const last = this.messages[this.messages.length - 1];
					if (last.role === "assistant" && !last.content) this.messages.pop();
					else if (last.role === "user") { this.messages.pop(); break; }
					else break;
				}
			}
			this.messages.push({ role: "user", content: text });
			this.messages.push({
				role: "assistant",
				content: buildCarbonSavingHeadline(this.currentRoute),
				carbonData: buildCarbonSavingPayload(this.currentRoute),
				carbonText: buildCarbonSavingReply(this.currentRoute), // a11y / fallback
			});
			return true;
		},
		/**
		 * 「附近的 X」/「沿途的 X」chip — 不打 LLM, 直接用已知座標查 POI 後 inject 結果。
		 * 使用順序: manualOrigin > currentRoute.start_coord > 都沒有就回 false 走原本 LLM 流程
		 * category: "park" | "restaurant" | "hotel" | "recycle" | "ubike"
		 */
		async injectNearbyPOIReply(text, category, { radiusKm = 1 } = {}) {
			const coord = this.manualOrigin
				|| (this.currentRoute?.start_coord && {
					lat: this.currentRoute.start_coord.lat ?? this.currentRoute.start_coord.Lat,
					lng: this.currentRoute.start_coord.lng ?? this.currentRoute.start_coord.Lng,
				});
			if (!coord || !Number.isFinite(coord.lat) || !Number.isFinite(coord.lng)) return false;

			const auth = useAuthStore();
			const token = auth?.token || localStorage.getItem("token") || "";
			if (!token) return false;

			// 維持 user/assistant 交替
			if (this.isStreaming) {
				this.cancelStream();
				while (this.messages.length > 0) {
					const last = this.messages[this.messages.length - 1];
					if (last.role === "assistant" && !last.content) this.messages.pop();
					else if (last.role === "user") { this.messages.pop(); break; }
					else break;
				}
			}
			this.messages.push({ role: "user", content: text });
			const assistantIndex = this.messages.length;
			this.messages.push({ role: "assistant", content: "查找中…" });

			try {
				const items = await fetchFindPOIs({
					lat: coord.lat,
					lng: coord.lng,
					radiusKm,
					categories: [category],
					token,
				});
				this.currentSearchPOIs = {
					categories: [category],
					center: coord,
					radius: radiusKm,
					items: items || [],
				};
				const labelMap = { park: "公園", restaurant: "環保餐廳", hotel: "環保旅館", recycle: "回收站", ubike: "YouBike 站點" };
				const label = labelMap[category] || category;
				if (!items?.length) {
					this.messages[assistantIndex].content = `半徑 ${radiusKm} km 內沒有找到${label}, 試試擴大範圍?`;
				} else {
					const lines = [`📍 ${this.currentRoute?.start_name || "你的起點"}附近的${label}有：`];
					items.slice(0, 10).forEach((p, i) => {
						lines.push(`${i + 1}. ${p.name || p.Name || "(未命名)"}`);
					});
					this.messages[assistantIndex].content = lines.join("\n");
				}
			} catch (e) {
				this.messages[assistantIndex].content = `查 POI 失敗: ${e.message}`;
			}
			return true;
		},
		// ---- 圖層開關 ----
		toggleLayer(key) {
			if (key in this.layerVisibility) {
				this.layerVisibility[key] = !this.layerVisibility[key];
			}
		},
		// ---- 地圖手動起終點 ----
		setPickMode(mode) {
			this.pickMode = ["origin", "destination"].includes(mode) ? mode : "idle";
		},
		setManualOrigin(coord, { silent = false } = {}) {
			this.manualOrigin = coord;
			if (!silent) this._maybeReplanFromManual();
		},
		setManualDest(coord, { silent = false } = {}) {
			this.manualDest = coord;
			if (!silent) this._maybeReplanFromManual();
		},
		clearManualPoints() {
			this.manualOrigin = null;
			this.manualDest = null;
			this.pickMode = "idle";
		},
		async _maybeReplanFromManual() {
			if (!this.manualOrigin || !this.manualDest) return;
			const auth = useAuthStore();
			const token = auth?.token || localStorage.getItem("token") || "";
			if (!token) {
				this.errorMessage = "請先登入再使用地圖起終點規劃";
				return;
			}
			try {
				const data = await fetchPlanRoute({
					originCoord: this.manualOrigin,
					destinationCoord: this.manualDest,
					token,
				});
				if (data) this.currentRoute = data;
			} catch (e) {
				this.errorMessage = `地圖起終點規劃失敗: ${e.message}`;
			}
		},
		// 處理路徑意圖, 含「目前位置」自動轉為 GPS 座標
		async _planFromIntent(intent, token) {
			const payload = { token };
			if (isCurrentLocationKeyword(intent.origin)) {
				try {
					payload.originCoord = await getCurrentPosition();
				} catch (e) {
					this.errorMessage = `無法取得你的位置：${e.message}。請點地圖選起點，或改用具體地名。`;
					return;
				}
			} else {
				payload.origin = intent.origin;
			}
			if (isCurrentLocationKeyword(intent.destination)) {
				try {
					payload.destinationCoord = await getCurrentPosition();
				} catch (e) {
					this.errorMessage = `無法取得你的位置：${e.message}。請點地圖選終點，或改用具體地名。`;
					return;
				}
			} else {
				payload.destination = intent.destination;
			}
			const data = await fetchPlanRoute(payload);
			if (data) {
				this.currentRoute = data;
				// 標記最後一筆 assistant 訊息: 路線已就緒, 附帶 routeData 內聯渲染
				const last = this.messages[this.messages.length - 1];
				if (last?.role === "assistant") {
					last.routeReady = true;
					last.routeData = data;
				}
			}
		},
		async _handleMeta(meta, token) {
			if (!meta) return;
			try {
				if (meta.intent === "plan_route") {
					await this._planFromIntent(
						{ origin: meta.params.origin, destination: meta.params.destination },
						token
					);
				} else if (meta.intent === "find_pois") {
					const lat = parseFloat(meta.params.lat);
					const lng = parseFloat(meta.params.lng);
					const radiusKm = parseFloat(meta.params.radius) || 1;
					const categories = (meta.params.categories || "")
						.split(",")
						.map((s) => s.trim())
						.filter(Boolean);
					const districts = (meta.params.districts || "")
						.split(",")
						.map((s) => s.trim())
						.filter(Boolean);
					if (!Number.isFinite(lat) || !Number.isFinite(lng) || !categories.length) return;
					const items = await fetchFindPOIs({ lat, lng, radiusKm, categories, districts, token });
					const withCoord = (items || []).filter((p) => Number.isFinite(p?.lat) && Number.isFinite(p?.lng));
					console.info("[eco] LLM META find_pois fetch:", {
						meta: meta.params,
						total: items?.length || 0,
						withCoord: withCoord.length,
					});
					// 若 LLM META 回的結果一筆有座標的也沒有, 但 FE detectPOIIntent 已成功設了 currentSearchPOIs,
					// 就不要覆寫掉 (避免 LLM 幻覺座標蓋掉 FE 真實搜尋結果)
					if (!withCoord.length && this.currentSearchPOIs?.items?.some?.((p) => Number.isFinite(p?.lat) && Number.isFinite(p?.lng))) {
						console.info("[eco] LLM META got 0 coord items, keep FE detectPOIIntent result");
						return;
					}
					this.currentSearchPOIs = {
						categories,
						districts,
						center: { lat, lng },
						radius: radiusKm,
						items: items || [],
					};
				}
			} catch (e) {
				console.warn("META handling failed:", e.message);
			}
		},
		_tryExtractRoute(text) {
			// 後備機制：若直接 fetch 失敗但 LLM 在文字中夾帶 JSON，嘗試抓 PlanResult
			if (this.currentRoute) return;
			const match = text.match(/\{[\s\S]*"routes"[\s\S]*\}/);
			if (!match) return;
			try {
				const parsed = JSON.parse(match[0]);
				if (Array.isArray(parsed.routes)) {
					this.currentRoute = parsed;
				}
			} catch {
				// 忽略解析失敗
			}
		},
	},
});
