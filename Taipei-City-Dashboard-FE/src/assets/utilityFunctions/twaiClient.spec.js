/**
 * twaiClient SSE/JSON tests using vitest's mock fetch.
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { chatTwai } from "./twaiClient.js";

afterEach(() => {
	vi.restoreAllMocks();
});

function mockJsonFetch(body) {
	global.fetch = vi.fn().mockResolvedValue({
		ok: true,
		headers: { get: () => "application/json" },
		json: async () => body,
	});
}

function mockSseFetch(events) {
	const encoder = new TextEncoder();
	const chunks = events.map((e) => encoder.encode(`data: ${e}\n\n`));
	const reader = { read: vi.fn() };
	chunks.forEach((c) => reader.read.mockResolvedValueOnce({ value: c, done: false }));
	reader.read.mockResolvedValueOnce({ value: undefined, done: true });
	global.fetch = vi.fn().mockResolvedValue({
		ok: true,
		headers: { get: () => "text/event-stream" },
		body: { getReader: () => reader },
	});
}

describe("chatTwai", () => {
	it("should call endpoint with bearer token and body", async () => {
		mockJsonFetch({ data: { content: "hi" } });
		const { promise } = chatTwai({
			messages: [{ role: "user", content: "你好" }],
			token: "JWT-XYZ",
			overrides: { stream: false },
		});
		await promise;
		expect(global.fetch).toHaveBeenCalledOnce();
		const [url, init] = global.fetch.mock.calls[0];
		expect(url).toMatch(/\/ai\/chat\/eco$/);
		expect(init.headers.Authorization).toBe("Bearer JWT-XYZ");
		const body = JSON.parse(init.body);
		expect(body.messages[0].content).toBe("你好");
	});

	it("should return content when non-streaming response", async () => {
		mockJsonFetch({ data: { content: "完整回覆" } });
		const onDone = vi.fn();
		const { promise } = chatTwai({
			messages: [],
			overrides: { stream: false },
			onDone,
		});
		const result = await promise;
		expect(result).toBe("完整回覆");
		expect(onDone).toHaveBeenCalledWith("完整回覆");
	});

	it("should emit chunks for SSE response (plain text)", async () => {
		mockSseFetch(["你", "好", "嗎"]);
		const chunks = [];
		const { promise } = chatTwai({
			messages: [],
			onChunk: (c) => chunks.push(c),
		});
		const full = await promise;
		expect(chunks).toEqual(["你", "好", "嗎"]);
		expect(full).toBe("你好嗎");
	});

	it("should extract generated_text from TWCC JSON chunks", async () => {
		mockSseFetch([
			'{"generated_text":"你","details":null,"finish_reason":null}',
			'{"generated_text":"好","details":null,"finish_reason":null}',
			'{"generated_text":"","details":null,"finish_reason":"stop"}',
		]);
		const chunks = [];
		const { promise } = chatTwai({
			messages: [],
			onChunk: (c) => chunks.push(c),
		});
		const full = await promise;
		expect(chunks).toEqual(["你", "好"]);
		expect(full).toBe("你好");
	});

	it("should reject and call onError on HTTP 500", async () => {
		global.fetch = vi.fn().mockResolvedValue({
			ok: false,
			status: 500,
			headers: { get: () => "" },
		});
		const onError = vi.fn();
		const { promise } = chatTwai({ messages: [], onError });
		await expect(promise).rejects.toThrow(/HTTP 500/);
		expect(onError).toHaveBeenCalled();
	});

	it("should support cancel via AbortController", async () => {
		global.fetch = vi.fn().mockImplementation(
			(_, init) =>
				new Promise((_, reject) => {
					init.signal.addEventListener("abort", () => reject(new Error("aborted")));
				})
		);
		const { promise, cancel } = chatTwai({ messages: [] });
		cancel();
		await expect(promise).rejects.toThrow(/aborted/);
	});
});
