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
 * 偵測「從 A 到 B」「A 走到 B」「A 至 B」之類的路徑規劃意圖。
 * 回 {origin, destination} 或 null。
 */
export function detectRouteIntent(text) {
	const patterns = [
		/從\s*([^到走至→]{2,20}?)\s*(?:走到|到|至|→)\s*([^的，。！？\s]{2,20})/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*走到\s*([^的，。！？\s]{2,20})$/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*→\s*([^的，。！？\s]{2,20})$/,
		/^([^到走至→\s的，。！？]{2,20}?)\s*至\s*([^的，。！？\s]{2,20})$/,
		// 「X到Y」沒有「從」，較寬鬆 — 整句必須是 X+到+Y 沒其他句綴
		/^([^到走至→\s的，。！？]{2,20}?)\s*到\s*([^的，。！？\s]{2,20})$/,
	];
	for (const re of patterns) {
		const m = text.match(re);
		if (m) return { origin: m[1].trim(), destination: m[2].trim() };
	}
	return null;
}

/**
 * 雙北區級中心座標 (供 POI 意圖偵測, 與 BE geocode 字典對齊大略值)
 * 不需要太精確 — 後端 find-pois 會以這個為圓心 + radius_km 過濾。
 */
const DISTRICT_COORDS = {
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
 * 偵測 POI 查詢意圖 (區 + 類別)。回 {center, radiusKm, categories, districts} 或 null。
 *
 * 範例:
 *   「信義區的環保餐廳」      → { center: 信義區, categories: [restaurant] }
 *   「大安區附近的回收站」    → { center: 大安區, categories: [recycle] }
 *   「台北車站附近的公園」    → 看不出區, 不偵測 (留給 LLM)
 */
export function detectPOIIntent(text) {
	if (!text) return null;
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
 * 從 LLM 回覆抽出最後一行的 [META:...] 標記。
 * 比 regex 偵測使用者意圖可靠：LLM 自己根據實際呼叫的工具產生。
 */
export function parseMetaMarker(text) {
	if (!text) return null;
	const lines = text.trim().split(/\r?\n/);
	for (let i = lines.length - 1; i >= 0; i--) {
		const m = lines[i].match(/^\s*\[META:([^\]]+)\]\s*$/);
		if (!m) continue;
		const parts = m[1].split("|");
		const intent = parts[0].trim();
		const params = {};
		for (const p of parts.slice(1)) {
			const eq = p.indexOf("=");
			if (eq < 0) continue;
			params[p.slice(0, eq).trim()] = p.slice(eq + 1).trim();
		}
		return { intent, params, raw: lines[i] };
	}
	return null;
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

/** 把 META 行從顯示文字中拿掉 */
export function stripMetaLine(text) {
	if (!text) return text;
	return text
		.split(/\r?\n/)
		.filter((l) => !/^\s*\[META:[^\]]+\]\s*$/.test(l))
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
			this.messages.push({ role: "user", content: text });
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

			// 並行呼叫: 路徑意圖直打 BE 拿結構化資料 (供 RouteCard / 未來地圖)
			const intent = detectRouteIntent(text);
			if (intent) {
				this._planFromIntent(intent, token).catch((e) => {
					console.warn("plan-route fetch failed:", e.message);
				});
			}

			// POI 意圖也直打 BE — 不依賴 LLM 加 META 標記, 確保地圖一定有 marker
			const poiIntent = detectPOIIntent(text);
			if (poiIntent) {
				fetchFindPOIs({
					lat: poiIntent.center.lat,
					lng: poiIntent.center.lng,
					radiusKm: poiIntent.radiusKm,
					categories: poiIntent.categories,
					districts: poiIntent.districts,
					token,
				})
					.then((items) => {
						this.currentSearchPOIs = {
							categories: poiIntent.categories,
							districts: poiIntent.districts,
							center: poiIntent.center,
							radius: poiIntent.radiusKm,
							items: items || [],
						};
					})
					.catch((e) => {
						console.warn("find-pois fetch failed:", e.message);
					});
			}

			// BE 端會自動注入 system prompt + tools, FE 只送 user/assistant 歷史
			// 為避免 TWCC 16k context 爆掉, 只送最後 6 輪 (12 則訊息)
			// 太舊的對話內容對當下查詢價值不大
			const HISTORY_CAP = 12;
			const all = this.messages.slice(0, -1);
			const trimmed = all.length > HISTORY_CAP ? all.slice(-HISTORY_CAP) : all;
			const apiMessages = trimmed.map((m) => ({ role: m.role, content: m.content }));

			this._currentStream = chatTwai({
				messages: apiMessages,
				session: this.session,
				token,
				onChunk: (chunk) => {
					this.messages[assistantIndex].content += chunk;
				},
				onDone: async (full) => {
					this.isStreaming = false;
					this._currentStream = null;
					const last = this.messages[this.messages.length - 1];
					if (last && last.role === "assistant" && !last.content) {
						// LLM 沒生成任何文字: 若 FE 已有路線, 顯示 fallback 摘要;
						// 否則直接移除空訊息
						if (last.routeReady && this.currentRoute) {
							last.content = buildFallbackSummary(this.currentRoute);
						} else {
							this.messages.pop();
							return;
						}
					}
					// 抓 LLM META 標記 -> 觸發對應 BE fetch
					const meta = parseMetaMarker(full);
					if (last && last.role === "assistant" && meta) {
						last.content = stripMetaLine(last.content);
					}
					await this._handleMeta(meta, token);
					// 後備: 仍試一次 regex extract
					if (!this.currentRoute) this._tryExtractRoute(full);
				},
				onError: (err) => {
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
					items.slice(0, 20).forEach((p, i) => {
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
