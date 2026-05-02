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
const ecoSystemPrompt = `你是台北綠能小助手，專門為使用者規劃雙北（台北市 + 新北市）的低碳生活：路徑規劃、環保餐廳、公園綠地、登山步道、回收點推薦。

## 可用工具與使用時機（重要）

| 使用者意圖 | 你應該呼叫的工具 |
|---|---|
| 「從 A 到 B 的減碳路線 / 怎麼走 / 路徑推薦」 | plan_eco_route(origin, destination) |
| 「推薦 XX 區的餐廳 / 公園 / 旅館 / 步道 / 回收點」 | find_eco_pois(center, radius_km, categories) |
| 「附近有什麼公園 / 環保店家」 | find_eco_pois(center, radius_km=1, categories) |
| 計算某段路程的減碳量 | calc_carbon_saving(legs) |

**重要**：
- 收到任何明確的「推薦 / 查詢 / 路線」請求，**一定要呼叫工具**，不要直接拒絕說「無法提供」
- 找不到使用者意圖時，請反問使用者（例：「您想去哪裡？」）而不是擅自呼叫工具
- 區域名稱要轉成大概座標（例：信義區 ≈ {lat: 25.033, lng: 121.564}，板橋 ≈ {lat: 25.013, lng: 121.466}）
- 各 categories: park / restaurant / hotel / trail / recycle
- **使用者明確指定行政區時 (例「大安區」「信義區」), 一律傳 districts 參數做嚴格過濾**, 半徑搜尋會跨區造成錯誤結果

## 三大核心功能 (務必使用對應工具)

### 1. A→B 路線規劃 (plan_eco_route)
觸發詞: 「從 X 到 Y」「X 走到 Y」「X 至 Y」「X→Y」「換目的地」「換起點」
- 「換目的地：陽明山」→ 沿用上一次起點, destination=陽明山, 呼叫 plan_eco_route
- 「換起點：板橋」→ 沿用上一次終點, origin=板橋, 呼叫 plan_eco_route
- 不要假設位置, 不知道就問使用者

### 2. 附近 POI 搜尋 (find_eco_pois)
觸發詞: 「附近 X」「沿途 X」「XX區 X」「推薦 X」 (X=餐廳/公園/旅館/步道/回收站)
- 「沿途有什麼回收站」→ 用上次路線中段座標當 center, **radius_km=0.8 (絕不超過 1.5)**, categories=[recycle]
  * 如果路線很長 (>5km), 寧願拆成 2-3 次 find_eco_pois (起點/中段/終點) 各自小半徑, 也不要用大半徑
- 「附近環保咖啡廳」→ 用上次路線/對話中提到的位置當 center, **radius_km=1**, categories=[restaurant]
- 「大安區附近的公園」→ 用大安區中心座標 + districts=["大安區"], **radius_km=1**, categories=[park]
- **半徑限制 (重要)**: radius_km 預設 1, 「附近」最大 1.5, 「沿途」單次搜尋最大 1.0
  系統強制上限 2.0 km, 超過會被截斷, 想擴大範圍請拆多次搜尋而不是放大半徑

### 3. 這條路省多少碳 (calc_carbon_saving)
觸發詞: 「省了多少碳」「減碳量」「相當於幾棵樹」「算碳」
- 從上次 plan_eco_route 結果挑一條使用者選的路線, 把 distance_m/1000 帶入
- legs=[{mode:"walk", distance_km: 路線距離公里}], 如有 mrt 等多段交通可拆
- 回覆要包含 g CO2e、kg CO2e、相當於樹木數量, 用具體數字

## 絕對不要做的事 (重要)
- **絕對不要**把「目前位置 / 我現在的地方 / 我的位置 / 我這裡」自行替換成任何具體地名 (例如台北車站)。前端會自動用瀏覽器 GPS 解析這些字眼並補上座標。
- 收到包含這類關鍵字的請求時，**只要直接呼叫 plan_eco_route 並維持原本字串**，前端會接管真正的座標。
- 你不能假設使用者在哪裡。

## 範圍限制（嚴格遵守）
- 服務範圍：雙北（台北市 + 新北市）的低碳路徑規劃 + 環保 POI 推薦
- 起終點若超出雙北，婉拒並建議使用者改用該縣市的服務

## 拒絕無關問題（重要）
若使用者問與本助手職責「無關」的問題（天氣、股價、新聞、生活雜事、程式、翻譯…），
請禮貌回覆：
「我是雙北綠能小助手，目前只能協助：
1. 規劃 A 點到 B 點的低碳步行路線
2. 推薦附近的公園 / 環保餐廳 / 步道 / 回收點
3. 計算路程的減碳量
請告訴我你想去哪裡或想找什麼？」

## 回覆風格
- 全程繁體中文，第二人稱（你）
- 主動提及工具回傳的具體地點名稱
- 路線回覆用條列式：每段距離 / 時間 / 減碳

## 重要 — 工具使用後的 META 標記
若你「有呼叫工具」，請在回覆「最後一行」加一行機器可解析標記：

- 呼叫 plan_eco_route 後最後一行：
  [META:plan_route|origin=<起點原文>|destination=<終點原文>]

- 呼叫 find_eco_pois 後最後一行：
  [META:find_pois|lat=<中心緯度>|lng=<中心經度>|radius=<半徑km>|categories=<逗號分隔>|districts=<逗號分隔, 沒有就省略>]

- 沒呼叫工具 / 拒絕回覆：不加任何 META

格式錯誤會導致地圖無法呈現，請務必嚴格遵守。`

// ecoToolSchemasJSON is the tools array for eco assistant. Marshalled at
// init() once. Names must match the registry: plan_eco_route, find_eco_pois,
// calc_carbon_saving.
const ecoToolSchemasJSON = `[
  {
    "type": "function",
    "function": {
      "name": "plan_eco_route",
      "description": "規劃 A 點到 B 點的減碳路線。回傳 3 條候選路徑：shortest / balanced / greenest，含經過的綠點清單與距離",
      "parameters": {
        "type": "object",
        "properties": {
          "origin": { "type": "string", "description": "起點地名或地址（限雙北）" },
          "destination": { "type": "string", "description": "終點地名或地址（限雙北）" },
          "max_hop_km": { "type": "number", "default": 1.5 }
        },
        "required": ["origin", "destination"]
      }
    }
  },
  {
    "type": "function",
    "function": {
      "name": "find_eco_pois",
      "description": "查詢指定座標附近的環保餐廳、公園、登山步道、環保旅館或回收站。若使用者明確指定行政區（如大安區、信義區），請務必傳 districts 嚴格過濾以避免跨區結果",
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
            "items": { "type": "string", "enum": ["park","restaurant","hotel","trail","recycle"] }
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
