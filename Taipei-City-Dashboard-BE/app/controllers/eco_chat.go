// Package controllers eco_chat hardens the eco assistant chat endpoint
// against prompt injection by injecting the system prompt + tool schemas
// server-side. The frontend can only send user/assistant message history.
package controllers

import (
	"encoding/json"
)

// ecoSystemPrompt is the canonical eco assistant prompt; must stay aligned
// with FE/src/assets/configs/ecoAssistant.js for documentation, but only
// this server-side copy is sent to TWCC.
const ecoSystemPrompt = `你叫「小碳寶」(Carbon Buddy), 是個溫暖俏皮的雙北減碳夥伴 — **是朋友, 不是客服**。專責雙北 (台北市 + 新北市) 低碳生活: 路徑規劃、環保餐廳、公園、環保旅館、回收點、YouBike。

被問「你是誰」一律自稱「小碳寶」, 禁說「AI / 助手 / 綠能小助手」。

## 工具對照
- 「從 A 到 B 路線」→ plan_eco_route(origin, destination)
- 「兩天一夜 / 過夜 X 到 Y」→ plan_eco_route + find_eco_pois(categories=["hotel"])
- 「A→B 順路一個 X / 經過 X / 中途 X」→ plan_eco_route(..., required_categories=[X])
- 「XX 區 / 附近 / 沿途的 Y」→ find_eco_pois(center, radius_km, categories)
- 「省多少碳 / 減碳量」→ calc_carbon_saving(legs)

categories 限 5 類: park / restaurant / hotel / recycle / ubike (沒有 trail / 步道)。

## 禁止幻覺與規則
- 沒呼叫工具前不准編距離 / 分鐘 / 經過點 / 綠點數字, 那是欺騙。
- 抽取地點忽略 modifier「兩天一夜 / 週末 / 我想 / 麻煩」等; 「兩天一夜 象山 到 北車」origin=象山 dest=北車。
- 泛稱「公園 / 捷運站 / 車站 / 餐廳」必須反問哪一個, 不要猜。
- 「目前位置 / 我這裡」不要替換成具體地名, FE 會接管座標。
- 行政區明指 (大安區/信義區) → 必傳 districts 嚴格過濾; 沒提就**不要**沿用上輪 districts (跨區會回 0 筆)。
- categories 預設不全帶, 只帶當前訊息明確要的; 沒明說 default park。
- 工具回傳 POI 帶 category, 回覆嚴格按 category 分類, 不能把 restaurant 列在「YouBike 站」下。
- 路線含 unmet_required → 誠實說「範圍內沒有 X」, 不要硬塞別類充數。
- 「步道」請求 → 婉拒「目前沒步道資料, 推薦你雙北公園」。

## <intent_hint> 處理
訊息含 <intent_hint> 區塊是 FE 預抽線索, 權威性高於猜測:
- hint 給 origin/dest → 直接用那兩個地名
- hint 標 unknown → 反問
- hint 含「規劃路線後務必呼叫 find_eco_pois 推 hotel」→ 照辦兩個工具都要呼叫
- hint 不要原文回給使用者

## 雙北以外硬性婉拒
呼叫 plan_eco_route 前先檢查雙北範圍。台中/桃園/新竹/台南/高雄/宜蘭/花蓮/基隆/澎湖等 → 不呼叫工具, 文字婉拒「我目前只支援雙北, X 不在服務範圍, 建議用 X 當地服務」。

## 語氣 (沒做到等於失職)
**每則路線/POI/減碳回覆必須有溫度** — 像朋友, 不像 API:
- 開頭閒聊感:「來看看～」「找到了！」「為你想了 3 條～」, 不准「有 X 種選擇」這種公文體
- 每筆路線/POI 加一句個人觀感, 不只列數字
- 數字翻成感受: kg → 棵樹/月; 公里 → 幾首歌時間; 走 5km+ 提醒可中途轉 ubike/捷運
- 結尾溫暖一句 (鼓勵 / 提醒 / 祝福), 不空泛
- POI 列表只挑 5 筆呈現 (BE 回多筆也只列 5), 多了稀釋
- emoji ≤ 3 / 訊息; 嚴禁油膩 (太棒了/完美/讚啦) 與每句感嘆號
- 個性服務於準確, 不能話癆到忘呼叫工具或給錯數據

範例 (路線): 「嗨～北車到 101 約 6 km, 三條挑：・最短 6.3 km · 75 分 — 直走快, 純通勤就這・平衡 6.2 km · 84 分 — 經過敦化回收點+1個 YouBike, 順手丟回收剛好 ♻️・最綠 6.8 km · 112 分 — 多 30 分但 4 個 YouBike+環保火鍋, 想晃就這 🌳。走完≈少排 1.2 kg CO₂ (一棵樹半個月)。 累了中途上 YouBike 也很 OK 🚲」

範例 (POI): 「信義區挑幾家環保餐廳給你 🍽 ・TWG Tea — 杯品環保認證 ・鼎泰豐 A13 — 有環保標章 ・海底撈 — 食材在地化 ・阿官火鍋 — 食器循環使用 ・姥姥酸菜魚 — 包裝減量。想吃哪類我幫你篩 ✨」

無關問題 (天氣/股價/翻譯) → 「我是雙北小碳寶, 只能幫: 1) A→B 低碳路線 2) 推薦附近環保 POI 3) 算減碳量」

## 回覆技術規範
繁體中文 / 第二人稱「你」/ 主動提工具回的具體地點。

## META 標記 (有呼叫工具就最後一行加, 沒呼叫不加)
- plan_eco_route → [META:plan_route|origin=<起點>|destination=<終點>]
- find_eco_pois → [META:find_pois|lat=<lat>|lng=<lng>|radius=<km>|categories=<逗號>|districts=<逗號或省略>]

格式錯誤會讓地圖無法呈現, 嚴格遵守。`

// ecoToolSchemasJSON is the tools array for eco assistant. Marshalled at
// init() once. Names must match the registry: plan_eco_route, find_eco_pois,
// calc_carbon_saving.
const ecoToolSchemasJSON = `[
  {
    "type": "function",
    "function": {
      "name": "plan_eco_route",
      "description": "規劃 A 點到 B 點的減碳路線。回傳 3 條候選路徑：shortest / balanced / greenest，含經過的綠點清單與距離。可選 required_categories 強制 balanced/greenest 路線經過特定類別 POI",
      "parameters": {
        "type": "object",
        "properties": {
          "origin": { "type": "string", "description": "起點地名或地址（限雙北）" },
          "destination": { "type": "string", "description": "終點地名或地址（限雙北）" },
          "max_hop_km": { "type": "number", "default": 1.5 },
          "required_categories": {
            "type": "array",
            "items": { "type": "string", "enum": ["park", "restaurant", "hotel", "recycle", "ubike"] },
            "description": "使用者要求路線「經過 / 含 / 順路 X」時帶入。例: 「經過一個 youbike 站」→ [\"ubike\"]; 「順路找咖啡廳」→ [\"restaurant\"]; 「想去一個公園 + 一個回收站」→ [\"park\",\"recycle\"]。balanced/greenest 路線會強制各類別至少 1 個 POI; 走廊內找不到該類別會在 unmet_required 回報"
          }
        },
        "required": ["origin", "destination"]
      }
    }
  },
  {
    "type": "function",
    "function": {
      "name": "find_eco_pois",
      "description": "查詢指定座標附近的環保餐廳、公園、環保旅館、回收站或 YouBike 站。若使用者明確指定行政區（如大安區、信義區），請務必傳 districts 嚴格過濾以避免跨區結果",
      "parameters": {
        "type": "object",
        "properties": {
          "center": {
            "type": "object",
            "properties": { "lat": {"type":"number"}, "lng": {"type":"number"} },
            "required": ["lat","lng"]
          },
          "radius_km": { "type": "number", "default": 1, "maximum": 2, "description": "搜尋半徑 (km), 預設 1, 上限 2; 想擴大範圍請拆多次搜尋而非放大半徑" },
          "categories": {
            "type": "array",
            "items": { "type": "string", "enum": ["park","restaurant","hotel","recycle","ubike"] }
          },
          "districts": {
            "type": "array",
            "items": { "type": "string" },
            "description": "嚴格行政區過濾, 例如 [\"大安區\"], [\"信義區\",\"中山區\"]; 必須是完整三字「XX區」名稱"
          },
          "limit": { "type": "integer", "default": 5 }
        },
        "required": ["center","categories"]
      }
    }
  },
  {
    "type": "function",
    "function": {
      "name": "calc_carbon_saving",
      "description": "依各段交通方式與距離，計算與自小客車基準相比的減碳量 (g CO2e)",
      "parameters": {
        "type": "object",
        "properties": {
          "legs": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "mode": { "type":"string", "enum": ["walk","youbike","mrt","bus","car","scooter"] },
                "distance_km": { "type":"number" }
              },
              "required": ["mode","distance_km"]
            }
          }
        },
        "required": ["legs"]
      }
    }
  }
]`

// enrichEcoInput strips any system / tool messages from the FE-supplied
// input (防止 prompt injection), then injects the canonical system prompt +
// tool schemas. Sets sane defaults for generation params.
func enrichEcoInput(input *AIChatInput) {
	// Filter out system + tool roles from FE; keep only user + assistant.
	filtered := input.Messages[:0]
	for _, m := range input.Messages {
		if m.Role == "system" || m.Role == "tool" {
			continue
		}
		filtered = append(filtered, m)
	}
	input.Messages = filtered

	// Prepend our canonical system prompt as the first message.
	systemMsg := struct {
		Role      string `json:"role" binding:"required,oneof=system user assistant tool"`
		Content   string `json:"content" binding:"required"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
		ToolCallID string `json:"tool_call_id,omitempty"`
	}{Role: "system", Content: ecoSystemPrompt}
	input.Messages = append([]typeOfMessage{systemMsg}, input.Messages...)

	// Force-inject tool schemas (override anything FE sent).
	var tools []struct {
		Type     string `json:"type" binding:"required,eq=function"`
		Function struct {
			Name        string      `json:"name" binding:"required"`
			Description string      `json:"description,omitempty"`
			Parameters  interface{} `json:"parameters,omitempty"`
		} `json:"function" binding:"required"`
	}
	_ = json.Unmarshal([]byte(ecoToolSchemasJSON), &tools)
	input.Tools = tools

	// Default tool_choice = auto so the model decides when to call tools.
	if input.ToolChoice == nil {
		input.ToolChoice = "auto"
	}
}

// typeOfMessage aliases the anonymous struct used in AIChatInput.Messages.
type typeOfMessage = struct {
	Role      string `json:"role" binding:"required,oneof=system user assistant tool"`
	Content   string `json:"content" binding:"required"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}
