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
import { detectRouteIntent, parseMetaMarker, stripMetaLine, useEcoAssistantStore } from "./ecoAssistantStore.js";

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

describe("stripMetaLine", () => {
	it("removes the META line from text", () => {
		const text = "三條路線給你...\n[META:plan_route|origin=A|destination=B]";
		expect(stripMetaLine(text)).toBe("三條路線給你...");
	});

	it("keeps text unchanged when no META", () => {
		expect(stripMetaLine("just text")).toBe("just text");
	});
});
