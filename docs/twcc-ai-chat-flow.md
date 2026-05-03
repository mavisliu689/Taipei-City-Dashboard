# TWCC AI Chat API - Tool Calling Flow

`POST /api/v1/ai/chat/twai`

```mermaid
flowchart TD
    U["使用者 (登入後)"] -->|"情境: 詢問綠能路線 / 查 POI / 推薦活動"| FE["前端 Vue Dashboard"]
    FE -->|"POST /api/v1/ai/chat/twai<br/>Authorization: Bearer JWT<br/>Body: messages + tools + tool_choice"| CTRL["controllers/ai.go<br/>(參數校驗 / session / SSE header)"]

    CTRL --> SEM{"Semaphore<br/>TWCC_MAX_CONCURRENT=10"}
    SEM -->|"超量"| BUSY["500 Server too busy"]
    SEM -->|"取得 slot"| SVC["services/ai/ai_service.go<br/>(狀態機 / token 統計)"]

    SVC -->|"langchaingo 呼叫"| TWCC["TWCC LLM<br/>llama3.3-ffm-70b-16k-chat"]
    TWCC -->|"回應"| PARSE{"是否含 tool_calls?<br/>(twcc.go cleanXML)"}

    PARSE -->|"否 - 純文字"| OUT["回前端"]
    PARSE -->|"是"| LOOP{"loop count < 5?"}
    LOOP -->|"否"| FORCE["強制收尾<br/>回最後文字"]
    LOOP -->|"是"| REG["tools/registry.go<br/>反射執行 Go function"]
    REG -->|"role: tool + tool_call_id<br/>塞回 messages"| SVC

    OUT -->|"stream=false"| JSON["JSON Response<br/>data.content / tool_used<br/>usage / latency_ms"]
    OUT -->|"stream=true"| SSE["SSE text/event-stream<br/>前 64 字緩衝攔截 tool call"]

    SVC -.->|"每筆寫入"| LOG[("ai_chatlog<br/>token / latency / ip")]

    JSON --> FE
    SSE --> FE
    BUSY --> FE
    FORCE --> JSON

    style SEM fill:#fff4e6,stroke:#ff922b
    style BUSY fill:#ffe3e3,stroke:#fa5252
    style LOOP fill:#fff4e6,stroke:#ff922b
    style PARSE fill:#e7f5ff,stroke:#339af0
    style TWCC fill:#f3f0ff,stroke:#7950f2
    style REG fill:#e6fcf5,stroke:#12b886
    style LOG fill:#f8f9fa,stroke:#868e96
```

## 輸入 (Request)

| 欄位 | 說明 |
|---|---|
| `Authorization` | `Bearer <JWT>`（需登入） |
| `messages` | `system / user / assistant / tool` 對話歷史 |
| `tools` | function schema 陣列（name / description / parameters） |
| `tool_choice` | `auto` / `none` / 指定函式 |
| `stream` | `true` 走 SSE，`false` 走 JSON |
| `temperature` / `top_p` / `top_k` / `max_new_tokens` / `seed` | 生成參數 |

## 輸出 (Response)

**非串流**

```json
{
  "status": "success",
  "data": {
    "content": "...",
    "tool_used": true,
    "session": "session_xxx",
    "model": "llama3.3-ffm-70b-16k-chat",
    "provider": "twcc",
    "latency_ms": 7176,
    "usage": { "input_tokens": 312, "output_tokens": 188, "total_tokens": 500 }
  }
}
```

**串流**：`text/event-stream`，前 64 字緩衝攔截 tool call，工具執行完才開始 stream 純文字。

## 限制

| 限制 | 來源 | 行為 |
|---|---|---|
| 並發 10 | `TWCC_MAX_CONCURRENT` Semaphore | 超量回 `500 Server too busy` |
| Tool loop 5 次 | `ai_service.go` 狀態機 | 超過強制收尾 |
| Timeout 60s | `TWCC_TIMEOUT` | 逾時報錯 |
| Retry 2 次 | `TWCC_MAX_RETRY`（僅非串流） | 自動重試 |
| SSE 前 64 字緩衝 | `providers/twcc/twcc.go` | 判斷 tool call 期間靜默 |

## 情境範例

1. **綠能路線**：使用者問「台北市府到新北市府的綠能路線」→ 模型呼叫 `get_eco_route` → 回填路線資料 → 模型生成自然語回覆。
2. **POI 查詢**：使用者問「附近的回收點」→ 模型呼叫 `search_recycle_poi` → 回 POI 清單。
3. **多輪 tool**：先 `get_user_location` → 再 `search_recycle_poi`（最多 5 輪）。
