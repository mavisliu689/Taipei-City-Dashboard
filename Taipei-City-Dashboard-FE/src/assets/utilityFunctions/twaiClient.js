/**
 * TWCC Chat Client — 封裝 POST /api/v1/ai/chat/twai 的 SSE 串流。
 *
 * 用法:
 *   const stream = chatTwai({ messages, tools, onChunk, onDone, onError });
 *   stream.cancel();   // 中途取消
 */

import { ECO_CHAT_DEFAULTS, ECO_CHAT_ENDPOINT } from "../configs/ecoAssistant.js";

/**
 * TWCC SSE chunk 可能是純文字或 JSON 包裝：
 *   {"generated_text": "你", "details": null, "finish_reason": null}
 *   或 OpenAI 風格 {"choices":[{"delta":{"content":"..."}}]}
 * 此函式抽出實際要顯示的字串，無法解析則回原文。
 */
function extractChunkText(payload) {
	if (!payload.startsWith("{")) return payload;
	try {
		const obj = JSON.parse(payload);
		if (typeof obj.generated_text === "string") return obj.generated_text;
		if (typeof obj.content === "string") return obj.content;
		const delta = obj?.choices?.[0]?.delta;
		if (delta && typeof delta.content === "string") return delta.content;
		// finish_reason 等控制訊息忽略
		return "";
	} catch {
		return payload;
	}
}

/**
 * @param {Object} params
 * @param {Array}  params.messages       — [{role, content}]
 * @param {Array}  [params.tools]        — TWCC function schemas
 * @param {string} [params.session]      — 對話 session id
 * @param {string} [params.token]        — JWT
 * @param {Object} [params.overrides]    — 覆寫 ECO_CHAT_DEFAULTS
 * @param {(chunk:string)=>void} [params.onChunk] — 每段 SSE data 觸發
 * @param {(full:string)=>void}  [params.onDone]  — 結束（拼接全文）
 * @param {(err:Error)=>void}    [params.onError] — 失敗
 * @returns {{ cancel: () => void, promise: Promise<string> }}
 */
export function chatTwai({
	messages,
	tools,
	session,
	token,
	overrides = {},
	onChunk,
	onDone,
	onError,
}) {
	const controller = new AbortController();
	const body = {
		...ECO_CHAT_DEFAULTS,
		...overrides,
		messages,
	};
	if (tools) body.tools = tools;
	if (session) body.session = session;

	const headers = { "Content-Type": "application/json" };
	if (token) headers.Authorization = `Bearer ${token}`;

	const promise = (async () => {
		let response;
		try {
			response = await fetch(ECO_CHAT_ENDPOINT, {
				method: "POST",
				headers,
				body: JSON.stringify(body),
				signal: controller.signal,
			});
		} catch (e) {
			if (onError) onError(e);
			throw e;
		}
		if (!response.ok) {
			const err = new Error(`TWCC chat HTTP ${response.status}`);
			if (onError) onError(err);
			throw err;
		}

		// 非串流：直接回 JSON.data.content
		const ctype = response.headers.get("content-type") || "";
		if (!ctype.includes("text/event-stream")) {
			const json = await response.json();
			const content = json?.data?.content || "";
			if (onChunk) onChunk(content);
			if (onDone) onDone(content);
			return content;
		}

		// 串流：逐 chunk 解析
		const reader = response.body.getReader();
		const decoder = new TextDecoder("utf-8");
		let buffer = "";
		let full = "";

		while (true) {
			const { value, done } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });

			// SSE 以 "\n\n" 為一筆事件
			let idx;
			while ((idx = buffer.indexOf("\n\n")) >= 0) {
				const event = buffer.slice(0, idx).trim();
				buffer = buffer.slice(idx + 2);

				// 取每行 data: 後面的內容
				const lines = event.split("\n");
				for (const line of lines) {
					if (!line.startsWith("data:")) continue;
					const payload = line.slice(5).trim();
					if (!payload || payload === "[DONE]") continue;
					const text = extractChunkText(payload);
					if (!text) continue;
					full += text;
					if (onChunk) onChunk(text);
				}
			}
		}

		if (onDone) onDone(full);
		return full;
	})();

	return { cancel: () => controller.abort(), promise };
}
