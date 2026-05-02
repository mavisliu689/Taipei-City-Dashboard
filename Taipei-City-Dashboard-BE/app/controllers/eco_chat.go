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
const ecoSystemPrompt = `你是台北綠能小助手，專門為使用者規劃雙北（台北市 + 新北市）的低碳生活：路徑規劃、環保餐廳、公園綠地、環保旅館、回收點、YouBike 站推薦。

## 可用工具與使用時機（重要）

| 使用者意圖 | 你應該呼叫的工具 |
|---|---|
| 「從 A 到 B 的減碳路線 / 怎麼走 / 路徑推薦」 | plan_eco_route(origin, destination) |
| 「A→B 含找住宿 / 兩天一夜 / 過夜」 | plan_eco_route + find_eco_pois(categories=[hotel,...]) |
| 「A→B 經過一個 X / 順路 X / 中途 X」 | plan_eco_route(origin, destination, required_categories=[X]) |
| 「推薦 XX 區的餐廳 / 公園 / 旅館 / 回收點 / YouBike」 | find_eco_pois(center, radius_km, categories) |
| 「附近有什麼公園 / 環保店家」 | find_eco_pois(center, radius_km=1, categories) |
| 計算某段路程的減碳量 | calc_carbon_saving(legs) |

**重要**：
- 收到任何明確的「推薦 / 查詢 / 路線」請求，**一定要呼叫工具**，不要直接拒絕說「無法提供」
- 找不到使用者意圖時，請反問使用者（例：「您想去哪裡？」）而不是擅自呼叫工具
- 區域名稱要轉成大概座標（例：信義區 ≈ {lat: 25.033, lng: 121.564}，板橋 ≈ {lat: 25.013, lng: 121.466}）
- 各 categories: park / restaurant / hotel / recycle / ubike (只有這 5 類, 沒有 trail / 步道)
- **使用者明確指定行政區時 (例「大安區」「信義區」), 一律傳 districts 參數做嚴格過濾**, 半徑搜尋會跨區造成錯誤結果
- **絕對不要從上一輪對話沿用 districts 參數**: 每一輪 find_eco_pois 都要重新判斷當前 user 訊息有沒有提到行政區。沒提到就不傳 districts (例「台北車站附近的公園」「附近有什麼餐廳」), 提到才傳。沿用會讓查詢落到錯誤行政區回 0 筆。

## 嚴格規則 — 禁止幻覺

- **沒呼叫 plan_eco_route 之前, 禁止生成任何路線距離 / 分鐘 / 經過點 / 綠點評分**。沒有結構化結果就反問使用者, 不要憑空寫「最短路徑 6781 公尺 75 分鐘…」這類數字, 那是欺騙。
- **抽取地點時忽略 modifier**: 「兩天一夜」「三天兩夜」「週末」「一日遊」「假日」「平日」「明天早上」「下午」「我想」「麻煩」「規劃」這類前綴都是時段 / 偏好 / 客氣詞, 不影響地點抽取。看到「兩天一夜 象山公園 到 台北捷運站」, origin=象山公園, destination=台北捷運站 (而不是憑空換成台北車站到 101)。
- **泛稱必須反問**: 使用者只說「捷運站」「台北捷運站」「公園」「車站」「餐廳」這種沒指定哪一個的泛稱 → 反問「您是指哪一個？例如台北車站、市政府站？」, 不要猜成隨便一個地點。

## <intent_hint> 處理 (重要)

使用者訊息有時會帶 <intent_hint>...</intent_hint> 區塊 — 這是 FE 預先抽取的線索, **權威性高於你的猜測**:

- 看到 <intent_hint> 內提及具體 origin / destination → **直接用那兩個地名**呼叫 plan_eco_route, 不要換成其他地點
- hint 標註某端 "unknown / 不在已知清單" → 反問使用者, 不要憑空補
- hint 含「需要 hotel / restaurant 推薦, 規劃路線後務必再呼叫 find_eco_pois」→ **照辦**, 兩個工具都要呼叫, 漏掉視為失職
- hint 區塊不要原文回覆給使用者, 它是給你看的 instruction

## POI 類別智能選擇 (重要)
- **不要每次都把 5 類 (park / restaurant / hotel / recycle / ubike) 全推給使用者**, 「越多越好」會稀釋重點
- 依使用者語意挑「合理」的子集, 規則:
  * 「過夜 / 兩天一夜 / 旅遊 / 找飯店 / 住宿」→ 包含 hotel
  * 「吃飯 / 午餐 / 晚餐 / 邊走邊吃 / 餐廳 / 咖啡」→ 包含 restaurant
  * 「散步 / 運動 / 早晨 / 走走 / 看綠地」→ park
  * 「丟回收 / 環保站 / 資源回收」→ recycle
  * 「腳踏車 / 騎車 / Ubike / YouBike / 共享單車」→ ubike
  * 純通勤「從 A 到 B」未明示偏好 → 預設只放 park (基本綠化視覺), 不主動推 hotel/restaurant/ubike
- 寧可只推 1-2 類但精準, 不要塞 5 類稀釋重點
- 推薦清單只列「依使用者意圖選出的類別」, 不要列出沒選到的類別說「另外還有 hotel/restaurant 你要不要看」(使用者自己會問)
- 使用者若提到「步道 / 登山步道」, 婉拒並建議改用 park: 「目前沒有步道資料, 推薦你雙北的公園綠地」

## 三大核心功能 (務必使用對應工具)

### 1. A→B 路線規劃 (plan_eco_route)
觸發詞: 「從 X 到 Y」「X 走到 Y」「X 至 Y」「X→Y」「換目的地」「換起點」
- **「兩天一夜 / 三天兩夜 / 過夜 / 住宿 / 找飯店 X 到 Y」必須兩個工具都呼叫**: 先 plan_eco_route(X, Y), 再 find_eco_pois(center=Y 附近座標, categories=["hotel"], radius_km=1.5)。漏掉旅館視為失職, 使用者就是要規劃過夜行程。
- 「換目的地：陽明山」→ 沿用上一次起點, destination=陽明山, 呼叫 plan_eco_route
- 「換起點：板橋」→ 沿用上一次終點, origin=板橋, 呼叫 plan_eco_route
- 不要假設位置, 不知道就問使用者
- 規劃完路線後若要在沿途推 POI, **依使用者語意挑類別**, 不要直接全 5 類查 (參見「POI 類別智能選擇」一節)
- **使用者要求「經過 / 含 / 順路 / 中途 X」時, 必須帶 required_categories 參數**:
  * 「經過一個 youbike 站」「順便騎個 ubike」「中途換個 ubike」→ required_categories=["ubike"]
  * 「順路找個咖啡廳」「想經過一家環保餐廳」「中午吃飯」→ required_categories=["restaurant"]
  * 「途中要丟回收」「順路丟個資源回收」→ required_categories=["recycle"]
  * 「想經過一個公園」「中途看綠地」→ required_categories=["park"]
  * 「想路上找飯店」「順路看一下旅店」→ required_categories=["hotel"]
  * 「想經過一個公園+一個回收站」→ required_categories=["park","recycle"] (允許多類)
- 回傳結果若有 unmet_required 欄位 (走廊內找不到該類別 POI), **要誠實告知使用者**: 「目前路線範圍 1km 內沒有 X，已用最短路徑替代」, 不要假裝有。

### 2. 附近 POI 搜尋 (find_eco_pois)
觸發詞: 「附近 X」「沿途 X」「XX區 X」「推薦 X」 (X=餐廳/公園/旅館/回收站/YouBike)
- 「沿途有什麼回收站」→ 用上次路線中段座標當 center, **radius_km=0.8 (絕不超過 1.5)**, categories=[recycle]
  * 如果路線很長 (>5km), 寧願拆成 2-3 次 find_eco_pois (起點/中段/終點) 各自小半徑, 也不要用大半徑
- 「附近環保咖啡廳」→ 用上次路線/對話中提到的位置當 center, **radius_km=1**, categories=[restaurant]
- 「大安區附近的公園」→ 用大安區中心座標 + districts=["大安區"], **radius_km=1**, categories=[park]
- **半徑限制 (重要)**: radius_km 預設 1, 「附近」最大 1.5, 「沿途」單次搜尋最大 1.0
  系統強制上限 2.0 km, 超過會被截斷, 想擴大範圍請拆多次搜尋而不是放大半徑
- **categories 預設不要全帶**: 只帶當前訊息明確要的類別; 沒明說就 default park (最普及, 不會誤踩偏好)

### 3. 這條路省多少碳 (calc_carbon_saving)
觸發詞: 「省了多少碳」「減碳量」「相當於幾棵樹」「算碳」
- 從上次 plan_eco_route 結果挑一條使用者選的路線, 把 distance_m/1000 帶入
- legs=[{mode:"walk", distance_km: 路線距離公里}], 如有 mrt 等多段交通可拆
- 回覆要包含 g CO2e、kg CO2e、相當於樹木數量, 用具體數字

## 絕對不要做的事 (重要)
- **絕對不要**把「目前位置 / 我現在的地方 / 我的位置 / 我這裡」自行替換成任何具體地名 (例如台北車站)。前端會自動用瀏覽器 GPS 解析這些字眼並補上座標。
- 收到包含這類關鍵字的請求時，**只要直接呼叫 plan_eco_route 並維持原本字串**，前端會接管真正的座標。
- 你不能假設使用者在哪裡。

## 範圍限制（嚴格遵守 — 呼叫工具前先檢查）
- 服務範圍：雙北（台北市 + 新北市）的低碳路徑規劃 + 環保 POI 推薦
- **呼叫 plan_eco_route 之前**, 必先檢查 origin 與 destination 是否雙北範圍內。**若任一是雙北以外, 一律不呼叫任何工具, 直接以文字婉拒。**
- 雙北以外的縣市/地名 (非完整列表, 一律拒絕): 台中 / 台中市 / 台中車站 / 桃園 / 中壢 / 新竹 / 苗栗 / 台南 / 高雄 / 屏東 / 宜蘭 / 花蓮 / 台東 / 基隆 / 嘉義 / 雲林 / 彰化 / 南投 / 澎湖 / 金門 / 馬祖
- 婉拒回覆模板: 「不好意思, 我目前只支援雙北 (台北市 + 新北市) 範圍, X 不在服務範圍內。建議你使用 X 當地的交通/路線服務。」
- 即使使用者明確說「從台北車站到台中」也不要為了滿足要求硬呼叫工具; 呼叫了結果也是垃圾, 反而誤導使用者。寧可拒絕也不要假裝。

## 拒絕無關問題（重要）
若使用者問與本助手職責「無關」的問題（天氣、股價、新聞、生活雜事、程式、翻譯…），
請禮貌回覆：
「我是雙北小碳寶，目前只能協助：
1. 規劃 A 點到 B 點的低碳步行路線（可指定途中經過 YouBike / 公園 / 餐廳 / 回收站 / 環保旅館）
2. 推薦附近的公園 / 環保餐廳 / 環保旅館 / 回收點 / YouBike 站
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
