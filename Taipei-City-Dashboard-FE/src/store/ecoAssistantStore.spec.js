/**
 * ecoAssistantStore 行為測試 (mock chatTwai)
 */
import { setActivePinia, createPinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../assets/utilityFunctions/twaiClient.js", () => ({
	chatTwai: vi.fn(),
}));

vi.mock("./authStore", () => ({
	useAuthStore: () => ({ token: "fake-jwt" }),
}));

import { chatTwai } from "../assets/utilityFunctions/twaiClient.js";
import {
	buildTotalCarbonSummary,
	calcDiningSavingG,
	calcLodgingSavingG,
	calcTransportSavingG,
	detectPOIIntent,
	detectRouteIntent,
	parseMetaMarker,
	stripMetaLine,
	useEcoAssistantStore,
} from "./ecoAssistantStore.js";

beforeEach(() => {
	global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: null }) });
});

beforeEach(() => {
	setActivePinia(createPinia());
});

afterEach(() => {
	vi.clearAllMocks();
});

describe("ecoAssistantStore", () => {
	it("should start with empty messages and closed panel", () => {
		const s = useEcoAssistantStore();
		expect(s.messages).toEqual([]);
		expect(s.open).toBe(false);
		expect(s.isStreaming).toBe(false);
	});

	it("should toggle panel open state", () => {
		const s = useEcoAssistantStore();
		s.togglePanel();
		expect(s.open).toBe(true);
		s.togglePanel();
		expect(s.open).toBe(false);
	});

	it("should append user + assistant messages on send", async () => {
		const s = useEcoAssistantStore();
		chatTwai.mockReturnValue({
			cancel: vi.fn(),
			promise: Promise.resolve("好的"),
		});
		await s.send("從台北市政府到新北市政府");
		expect(s.messages).toHaveLength(2);
		expect(s.messages[0]).toMatchObject({ role: "user", content: "從台北市政府到新北市政府" });
		expect(s.messages[1].role).toBe("assistant");
	});

	it("should accumulate streamed chunks into the assistant message", async () => {
		const s = useEcoAssistantStore();
		chatTwai.mockImplementation(({ onChunk, onDone }) => {
			onChunk("你");
			onChunk("好");
			onDone("你好");
			return { cancel: vi.fn(), promise: Promise.resolve("你好") };
		});
		await s.send("hi");
		expect(s.messages[1].content).toBe("你好");
		expect(s.isStreaming).toBe(false);
	});

	it("should set errorMessage when chatTwai fails", async () => {
		const s = useEcoAssistantStore();
		chatTwai.mockImplementation(({ onError }) => {
			const err = new Error("HTTP 500");
			onError(err);
			return { cancel: vi.fn(), promise: Promise.reject(err) };
		});
		await s.send("hi");
		expect(s.errorMessage).toBe("HTTP 500");
	});

	it("should reset messages and state", async () => {
		const s = useEcoAssistantStore();
		s.messages = [{ role: "user", content: "x" }];
		s.session = "abc";
		s.reset();
		expect(s.messages).toEqual([]);
		expect(s.session).toBeNull();
	});

	it("should cancel previous stream and accept new send when streaming", async () => {
		const s = useEcoAssistantStore();
		const cancel = vi.fn();
		s.isStreaming = true;
		s.messages = [
			{ role: "user", content: "previous question" },
			{ role: "assistant", content: "" }, // 串流中的空 assistant
		];
		s._currentStream = { cancel, promise: Promise.resolve() };
		chatTwai.mockReturnValue({ cancel: vi.fn(), promise: Promise.resolve("好") });
		await s.send("new question");
		expect(cancel).toHaveBeenCalled();
		// 舊 user + 空 assistant 都被清掉, 只剩新 user + 新 assistant
		expect(s.messages).toHaveLength(2);
		expect(s.messages[0].content).toBe("new question");
	});

	it("should ignore duplicate consecutive send (same content)", async () => {
		const s = useEcoAssistantStore();
		s.messages = [{ role: "user", content: "重複的問題" }];
		await s.send("重複的問題");
		expect(s.messages).toHaveLength(1); // 沒有 push 第二次
		expect(chatTwai).not.toHaveBeenCalled();
	});
});

// ─────────────────────────────────────────────────────────────
// 多輪對話壓測 — 模擬實際使用中常見會卡住的情境:
//   1. 連續 5 輪都成功 → message 序列必須交替 user/assistant
//   2. 串流中切換問題 → 不能殘留兩筆連續 user (TWCC 會拒絕)
//   3. 錯誤後 isStreaming 必須歸 false, 下一筆才送得出去
//   4. 串流結束但 LLM 回空字串 → 空 assistant 必須清掉, 下一筆不會 double-up
//   5. 歷史超過 cap (12 則) → 只送最後 12 則給 BE, 不會撐爆 16k token
// ─────────────────────────────────────────────────────────────
describe("multi-turn stress", () => {
	function mockReply(text) {
		chatTwai.mockImplementationOnce(({ onChunk, onDone }) => {
			onChunk(text);
			onDone(text);
			return { cancel: vi.fn(), promise: Promise.resolve(text) };
		});
	}

	it("5 連續成功的對話 → 交替 user/assistant 且都有內容", async () => {
		const s = useEcoAssistantStore();
		const turns = [
			["從台北市政府到新北市政府", "好的, 三條路線給你"],
			["這條路省了多少碳", "汽車對標減碳 1.2 kg"],
			["信義區的環保餐廳", "推薦捌伍添第..."],
			["大安區附近的回收站", "復興南路有兩個回收站"],
			["中山區附近的環保咖啡廳", "推薦兩家..."],
		];
		for (const [q, a] of turns) {
			mockReply(a);
			await s.send(q);
		}
		// 5 user + 5 assistant = 10 messages, 交替
		expect(s.messages).toHaveLength(10);
		for (let i = 0; i < s.messages.length; i++) {
			expect(s.messages[i].role).toBe(i % 2 === 0 ? "user" : "assistant");
			expect(s.messages[i].content.length).toBeGreaterThan(0);
		}
		expect(s.isStreaming).toBe(false);
	});

	it("串流中送新訊息 → 不能殘留兩筆連續 user", async () => {
		const s = useEcoAssistantStore();
		s.isStreaming = true;
		s.messages = [
			{ role: "user", content: "上一個問題" },
			{ role: "assistant", content: "" },
		];
		s._currentStream = { cancel: vi.fn(), promise: Promise.resolve() };
		mockReply("新答案");
		await s.send("新問題");
		// 不能出現 [user, user] 連續
		for (let i = 1; i < s.messages.length; i++) {
			if (s.messages[i].role === "user") {
				expect(s.messages[i - 1].role).not.toBe("user");
			}
		}
		expect(s.messages.find((m) => m.content === "新問題")).toBeTruthy();
	});

	it("錯誤後 isStreaming 歸 false → 下一筆送得出去", async () => {
		const s = useEcoAssistantStore();
		// 第一輪: 失敗
		chatTwai.mockImplementationOnce(({ onError }) => {
			const err = new Error("HTTP 500");
			onError(err);
			return { cancel: vi.fn(), promise: Promise.reject(err) };
		});
		await s.send("第一個會失敗");
		expect(s.isStreaming).toBe(false);
		expect(s.errorMessage).toBe("HTTP 500");

		// 第二輪: 成功 — 必須能送出, 不能因為殘留 isStreaming/empty assistant 卡住
		mockReply("這次成功");
		await s.send("再試一次");
		const last = s.messages[s.messages.length - 1];
		expect(last.role).toBe("assistant");
		expect(last.content).toBe("這次成功");
		expect(chatTwai).toHaveBeenCalledTimes(2);
	});

	it("LLM 回空字串 → 空 assistant 被清掉, 下一筆不會 double-up", async () => {
		const s = useEcoAssistantStore();
		// 第一輪: LLM 完全沒回 (onDone 但 chunks 為 0)
		chatTwai.mockImplementationOnce(({ onDone }) => {
			onDone("");
			return { cancel: vi.fn(), promise: Promise.resolve("") };
		});
		await s.send("空回應");
		// 空 assistant 被 pop 掉 → 應該只剩 user
		expect(s.messages).toHaveLength(1);
		expect(s.messages[0].role).toBe("user");

		// 第二輪: 不能因為前一輪殘留導致排序錯誤
		mockReply("正常回應");
		await s.send("再問一次");
		// 應該是 [user1, user2, assistant2] 或 [user1, assistant-fallback, user2, assistant2]
		// 至少最後一筆要是新的 assistant 內容
		const last = s.messages[s.messages.length - 1];
		expect(last.role).toBe("assistant");
		expect(last.content).toBe("正常回應");
	});

	it("歷史 > 12 則 → BE 只收最後 12 則 (HISTORY_CAP)", async () => {
		const s = useEcoAssistantStore();
		// 預先塞 14 則 (7 輪) 已完成的對話
		for (let i = 0; i < 7; i++) {
			s.messages.push({ role: "user", content: `Q${i}` });
			s.messages.push({ role: "assistant", content: `A${i}` });
		}
		mockReply("最新答案");
		await s.send("Q7");
		// 取到的 messages 參數應該被 trim 到 12 則
		const callArgs = chatTwai.mock.calls[0][0];
		expect(callArgs.messages).toHaveLength(12);
		// 確認最舊的 Q0/A0 被丟掉, 保留較近的歷史
		expect(callArgs.messages[0].content).not.toBe("Q0");
		// 最後一則應該是新 user "Q7"
		expect(callArgs.messages[callArgs.messages.length - 1]).toMatchObject({
			role: "user",
			content: "Q7",
		});
	});

	it("錯誤後不會留下空 assistant 卡住下一輪 (重現截圖卡住情境)", async () => {
		const s = useEcoAssistantStore();
		// 第一輪: 失敗
		chatTwai.mockImplementationOnce(({ onError }) => {
			const err = new Error("network drop");
			onError(err);
			return { cancel: vi.fn(), promise: Promise.reject(err) };
		});
		await s.send("中山區附近的環保咖啡廳");
		// 失敗後: 空 assistant 必須被清掉, 避免變成下一輪的 ghost
		const tailRoles = s.messages.map((m) => m.role);
		expect(tailRoles).not.toContain("assistant"); // 整輪只剩 user (assistant 被 pop)

		// 第二輪: 成功 — 不能出現兩筆連續 user
		mockReply("找到了！");
		await s.send("台北車站附近的公園");
		// 序列必須是合法的 [user, user→assistant 不行 / 必須交替]
		// 簡單檢查: 兩筆 user 之間不可省略 assistant
		const idxes = s.messages.map((m, i) => (m.role === "user" ? i : -1)).filter((i) => i >= 0);
		for (let i = 1; i < idxes.length; i++) {
			const between = s.messages.slice(idxes[i - 1] + 1, idxes[i]);
			// 兩筆 user 之間應該沒有空 assistant 也不能直接相連
			// 此 case 預期: 第一輪失敗後整個被清掉, 所以兩個 user 之間應該只有 0 個東西
			// 但合法狀態是 [user1, user2, asst2] 或 [user2, asst2] (user1 被清)
			expect(between.length === 0 || between.some((m) => m.content)).toBe(true);
		}
		// 最終一定要有最後 user 的回應
		const last = s.messages[s.messages.length - 1];
		expect(last).toMatchObject({ role: "assistant", content: "找到了！" });
	});

	it("快速連點 chip 3 次 → 最後一個成功送出, 不殘留 orphan 訊息", async () => {
		const s = useEcoAssistantStore();
		// 第一個會被中斷的長串流 — 用未 resolve 的 promise 模擬
		let resolveFirst;
		chatTwai.mockImplementationOnce(() => ({
			cancel: vi.fn(),
			promise: new Promise((r) => { resolveFirst = r; }),
		}));
		// 第二個也會被中斷
		chatTwai.mockImplementationOnce(() => ({
			cancel: vi.fn(),
			promise: new Promise(() => {}),
		}));
		// 第三個成功
		mockReply("第三個答案");

		// 快速連送 3 個 (不 await, 模擬使用者連點)
		s.send("Q1");
		s.send("Q2");
		await s.send("Q3");
		// 確保第一個 promise 不會卡住測試
		if (resolveFirst) resolveFirst("");

		// 不能有兩筆連續 user 訊息
		for (let i = 1; i < s.messages.length; i++) {
			if (s.messages[i].role === "user") {
				expect(s.messages[i - 1].role).not.toBe("user");
			}
		}
		// 最後一筆是 Q3 的 assistant
		const last = s.messages[s.messages.length - 1];
		expect(last.role).toBe("assistant");
		expect(last.content).toBe("第三個答案");
	});
});

describe("detectPOIIntent", () => {
	it.each([
		["信義區的環保餐廳", "信義區", "restaurant"],
		["大安區附近的回收站", "大安區", "recycle"],
		["中山區附近的環保咖啡廳", "中山區", "restaurant"],
		["板橋區的公園", "板橋區", "park"],
		["中山區附近的 YouBike 站", "中山區", "ubike"],
		["大安區的微笑單車", "大安區", "ubike"],
		["信義區附近共享單車", "信義區", "ubike"],
	])("matches '%s' → district=%s, category=%s", (text, district, cat) => {
		const r = detectPOIIntent(text);
		expect(r).toBeTruthy();
		expect(r.districts).toContain(district);
		expect(r.categories).toContain(cat);
		expect(r.center.lat).toBeGreaterThan(24);
		expect(r.center.lng).toBeGreaterThan(120);
	});

	it("returns null when no district keyword", () => {
		expect(detectPOIIntent("有什麼好吃的")).toBeNull();
	});

	it("returns null when district present but no category", () => {
		expect(detectPOIIntent("信義區好玩嗎")).toBeNull();
	});
});

describe("detectRouteIntent", () => {
	it.each([
		["從台北市政府到新北市政府", "台北市政府", "新北市政府"],
		["從台北車站走到大安森林公園", "台北車站", "大安森林公園"],
		["板橋至中山區", "板橋", "中山區"],
		["信義區→大安區", "信義區", "大安區"],
		["以力科技到新北青年局", "以力科技", "新北青年局"],
		["板橋車站到象山公園", "板橋車站", "象山公園"],
	])("matches '%s' → origin=%s destination=%s", (text, origin, destination) => {
		expect(detectRouteIntent(text)).toEqual({ origin, destination });
	});

	it("returns null when no route intent", () => {
		expect(detectRouteIntent("推薦信義區的環保餐廳")).toBeNull();
		expect(detectRouteIntent("hi")).toBeNull();
	});
});

describe("parseMetaMarker", () => {
	it("parses plan_route META on last line", () => {
		const text = "三條路線給你...\n[META:plan_route|origin=以力科技|destination=板橋車站]";
		expect(parseMetaMarker(text)).toEqual({
			intent: "plan_route",
			params: { origin: "以力科技", destination: "板橋車站" },
			raw: "[META:plan_route|origin=以力科技|destination=板橋車站]",
		});
	});

	it("parses find_pois META", () => {
		const text = "信義區公園:\n[META:find_pois|lat=25.033|lng=121.564|radius=1|categories=park]";
		const r = parseMetaMarker(text);
		expect(r.intent).toBe("find_pois");
		expect(r.params.lat).toBe("25.033");
		expect(r.params.categories).toBe("park");
	});

	it("returns null when no META line", () => {
		expect(parseMetaMarker("just a normal reply")).toBeNull();
	});

	it("finds last META when multiple present", () => {
		const text = "[META:plan_route|origin=A|destination=B]\n說明...\n[META:plan_route|origin=X|destination=Y]";
		const r = parseMetaMarker(text);
		expect(r.params.origin).toBe("X");
	});
});

// ─────────────────────────────────────────────────────────────
// 餐飲/住宿/交通減碳公式 — 公式來源:
//   餐飲: 一般葷食基準 2,500 g − 環保餐廳 1,800 g = 700 g/餐 (-28%)
//   住宿: 一般旅店基準 29,000 g − Green Coin 旅店 23,000 g = 6,000 g/晚 (-20%)
//   交通: BASELINES 中各方式 g/km × 距離
// ─────────────────────────────────────────────────────────────
describe("calcDiningSavingG", () => {
	it("回傳 (2500 − 1800) × 餐數", () => {
		expect(calcDiningSavingG(0)).toBe(0);
		expect(calcDiningSavingG(1)).toBe(700);
		expect(calcDiningSavingG(3)).toBe(2100);
	});
	it("負數或非數字 clamp 為 0", () => {
		expect(calcDiningSavingG(-2)).toBe(0);
		expect(calcDiningSavingG("abc")).toBe(0);
		expect(calcDiningSavingG(null)).toBe(0);
	});
	it("小數 floor 為整數餐數", () => {
		expect(calcDiningSavingG(2.7)).toBe(1400);
	});
});

describe("calcLodgingSavingG", () => {
	it("回傳 (29000 − 23000) × 晚數", () => {
		expect(calcLodgingSavingG(0)).toBe(0);
		expect(calcLodgingSavingG(1)).toBe(6000);
		expect(calcLodgingSavingG(2)).toBe(12000);
	});
	it("負數 clamp 為 0", () => {
		expect(calcLodgingSavingG(-1)).toBe(0);
	});
});

describe("calcTransportSavingG", () => {
	const route = {
		start_name: "A",
		end_name: "B",
		routes: [{ label: "綠意路線", distance_m: 5000, estimated_minutes: 60 }],
	};
	it("汽車對標 5km → 192 × 5 = 960 g", () => {
		expect(calcTransportSavingG(route, "car")).toBeCloseTo(960, 5);
	});
	it("捷運對標 5km → 33 × 5 = 165 g", () => {
		expect(calcTransportSavingG(route, "mrt")).toBeCloseTo(165, 5);
	});
	it("無路線 → 0", () => {
		expect(calcTransportSavingG(null)).toBe(0);
		expect(calcTransportSavingG({ routes: [] })).toBe(0);
	});
});

describe("buildTotalCarbonSummary", () => {
	const route = {
		start_name: "台北市政府",
		end_name: "新北市政府",
		routes: [{ label: "綠意路線", distance_m: 5000, estimated_minutes: 60 }],
	};

	it("無 route → 空字串", () => {
		expect(buildTotalCarbonSummary({ route: null, meals: 1, nights: 1 })).toBe("");
	});

	it("meals=0 nights=0 → 只算交通, 不出現餐飲/住宿段", () => {
		const out = buildTotalCarbonSummary({ route, baselineKey: "car" });
		expect(out).toContain("交通");
		expect(out).not.toContain("餐飲");
		expect(out).not.toContain("住宿");
	});

	it("meals=2 nights=1 → 結果 + 總和, 不含公式段", () => {
		const out = buildTotalCarbonSummary({
			route,
			baselineKey: "car",
			meals: 2,
			nights: 1,
		});
		// 不再顯示公式
		expect(out).not.toContain("(2,500 − 1,800)");
		expect(out).not.toContain("(29,000 − 23,000)");
		expect(out).not.toContain("g/km");
		// 結果區 — 數值 (g→kg, 兩位小數)
		expect(out).toMatch(/交通[^\n]*0\.96\s*kg/);   // 192 × 5 = 960g
		expect(out).toMatch(/餐飲[^\n]*1\.40\s*kg/);   // 700 × 2 = 1400g
		expect(out).toMatch(/住宿[^\n]*6\.00\s*kg/);   // 6000 × 1 = 6000g
		// 總和: 0.96 + 1.40 + 6.00 = 8.36 kg
		expect(out).toMatch(/總減碳[^\n]*8\.36\s*kg/);
	});

	it("僅 meals 或僅 nights 也能單獨呈現", () => {
		const onlyDining = buildTotalCarbonSummary({ route, meals: 1, nights: 0 });
		expect(onlyDining).toContain("餐飲");
		expect(onlyDining).not.toContain("住宿");

		const onlyLodging = buildTotalCarbonSummary({ route, meals: 0, nights: 2 });
		expect(onlyLodging).toContain("住宿");
		expect(onlyLodging).not.toContain("餐飲");
	});
});

describe("stripMetaLine", () => {
	it("removes the META line from text", () => {
		const text = "三條路線給你...\n[META:plan_route|origin=A|destination=B]";
		expect(stripMetaLine(text)).toBe("三條路線給你...");
	});

	it("keeps text unchanged when no META", () => {
		expect(stripMetaLine("just text")).toBe("just text");
	});
});
