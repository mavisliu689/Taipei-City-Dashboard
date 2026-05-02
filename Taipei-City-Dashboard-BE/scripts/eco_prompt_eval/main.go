// Standalone smoke-test for the eco assistant system prompt.
// Reads ecoSystemPrompt + ecoToolSchemasJSON straight from
// app/controllers/eco_chat.go (string extraction) so this script always
// exercises the live prompt without duplicating it.
//
// Usage:
//   cd Taipei-City-Dashboard-BE
//   TWCC_API_KEY=xxx go run ./scripts/eco_prompt_eval
//   # or rely on .env autoload below.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	chatPath  = "/models/conversation"
	defaultBaseURL = "https://api-ams.twcc.ai/api"
	defaultModel   = "llama3.3-ffm-70b-32k-chat"
)

type msg struct {
	Role    string  `json:"role"`
	Content *string `json:"content"`
	Name    string  `json:"name,omitempty"`
}

type tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description,omitempty"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

type req struct {
	Model      string                 `json:"model"`
	Messages   []msg                  `json:"messages"`
	Parameters map[string]interface{} `json:"parameters"`
	Tools      []tool                 `json:"tools,omitempty"`
	ToolChoice string                 `json:"tool_choice,omitempty"`
}

type resp struct {
	GeneratedText string `json:"generated_text"`
	ToolCalls     []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls,omitempty"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type testCase struct {
	id       string
	desc     string
	history  []msg
	userMsg  string
	expect   string
	checkFn  func(toolName string, args map[string]interface{}, finalText string) (bool, string)
}

func textMsg(role, content string) msg {
	return msg{Role: role, Content: &content}
}

func extractPromptAndTools(eco string) (string, []tool, error) {
	const promptKey = "const ecoSystemPrompt = `"
	pStart := strings.Index(eco, promptKey)
	if pStart < 0 {
		return "", nil, fmt.Errorf("prompt const not found")
	}
	pStart += len(promptKey)
	pEnd := strings.Index(eco[pStart:], "`")
	if pEnd < 0 {
		return "", nil, fmt.Errorf("prompt closing backtick not found")
	}
	prompt := eco[pStart : pStart+pEnd]

	const toolsKey = "const ecoToolSchemasJSON = `"
	tStart := strings.Index(eco, toolsKey)
	if tStart < 0 {
		return prompt, nil, fmt.Errorf("tools const not found")
	}
	tStart += len(toolsKey)
	tEnd := strings.Index(eco[tStart:], "`")
	if tEnd < 0 {
		return prompt, nil, fmt.Errorf("tools closing backtick not found")
	}
	rawTools := eco[tStart : tStart+tEnd]
	var tools []tool
	if err := json.Unmarshal([]byte(rawTools), &tools); err != nil {
		return prompt, nil, fmt.Errorf("decode tools: %w", err)
	}
	return prompt, tools, nil
}

func loadEnvFromFile(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

func callTWCC(ctx context.Context, baseURL, apiKey, model string, prompt string, tools []tool, history []msg, userMsg string) (string, map[string]interface{}, string, error) {
	msgs := []msg{textMsg("system", prompt)}
	msgs = append(msgs, history...)
	msgs = append(msgs, textMsg("user", userMsg))

	body := req{
		Model:    model,
		Messages: msgs,
		Parameters: map[string]interface{}{
			"max_new_tokens": 600,
			"temperature":    0.01,
		},
		Tools:      tools,
		ToolChoice: "auto",
	}
	jsonBody, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+chatPath, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-KEY", apiKey)

	httpClient := &http.Client{Timeout: 60 * time.Second}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", nil, "", err
	}
	defer httpResp.Body.Close()

	rawBody, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return "", nil, "", fmt.Errorf("status %d: %s", httpResp.StatusCode, string(rawBody))
	}

	var r resp
	if err := json.Unmarshal(rawBody, &r); err != nil {
		return "", nil, "", fmt.Errorf("decode resp: %w; raw=%s", err, string(rawBody))
	}

	var toolName, rawArgs, finalText string
	if len(r.ToolCalls) > 0 {
		toolName = r.ToolCalls[0].Function.Name
		rawArgs = r.ToolCalls[0].Function.Arguments
	} else if len(r.Choices) > 0 && len(r.Choices[0].Message.ToolCalls) > 0 {
		toolName = r.Choices[0].Message.ToolCalls[0].Function.Name
		rawArgs = r.Choices[0].Message.ToolCalls[0].Function.Arguments
	}
	if r.GeneratedText != "" {
		finalText = r.GeneratedText
	} else if len(r.Choices) > 0 {
		finalText = r.Choices[0].Message.Content
	}

	args := map[string]interface{}{}
	if rawArgs != "" {
		_ = json.Unmarshal([]byte(rawArgs), &args)
	}
	return toolName, args, finalText, nil
}

func categoriesFromArgs(args map[string]interface{}) []string {
	v, ok := args["categories"]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func setEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

func setSubset(want, got []string) bool {
	m := map[string]bool{}
	for _, x := range got {
		m[x] = true
	}
	for _, x := range want {
		if !m[x] {
			return false
		}
	}
	return true
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func cases() []testCase {
	return []testCase{
		{
			id:   "C1",
			desc: "純通勤 A→B (預期: plan_eco_route, 不主動呼叫 find_eco_pois)",
			userMsg: "從台北車站到陽明山",
			expect:  "plan_eco_route, 沒有 categories 多選",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				return true, "OK plan_eco_route"
			},
		},
		{
			id:   "C2",
			desc: "兩天一夜旅遊 (預期: plan_eco_route, 後續會推 hotel)",
			userMsg: "我們明天兩天一夜，從台北市政府走到淡水",
			expect:  "plan_eco_route 為主, 並提及住宿",
			checkFn: func(name string, _ map[string]interface{}, text string) (bool, string) {
				ok := name == "plan_eco_route" || (name == "find_eco_pois")
				return ok, fmt.Sprintf("called %q, text mentions hotel=%v", name, contains(text, "hotel") || contains(text, "住宿") || contains(text, "旅館"))
			},
		},
		{
			id:   "C3",
			desc: "明確找餐廳 (預期: find_eco_pois, categories=[restaurant])",
			userMsg: "中山區附近的環保餐廳",
			expect:  "find_eco_pois, categories=[restaurant], districts=[中山區]",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "find_eco_pois" {
					return false, fmt.Sprintf("expected find_eco_pois, got %q", name)
				}
				cats := categoriesFromArgs(args)
				if !setEqual(cats, []string{"restaurant"}) {
					return false, fmt.Sprintf("categories=%v, want [restaurant]", cats)
				}
				return true, fmt.Sprintf("categories=%v", cats)
			},
		},
		{
			id:   "C4",
			desc: "散步意圖 (預期: find_eco_pois, categories ⊇ [park] 或 [park,trail])",
			userMsg: "週末想去散步，中正區有什麼推薦",
			expect:  "find_eco_pois, categories 含 park, 不含 hotel/restaurant",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "find_eco_pois" {
					return false, fmt.Sprintf("expected find_eco_pois, got %q", name)
				}
				cats := categoriesFromArgs(args)
				okPark := setSubset([]string{"park"}, cats)
				hasHotel := setSubset([]string{"hotel"}, cats)
				hasRest := setSubset([]string{"restaurant"}, cats)
				if !okPark || hasHotel || hasRest {
					return false, fmt.Sprintf("categories=%v (want park, no hotel/restaurant)", cats)
				}
				return true, fmt.Sprintf("categories=%v", cats)
			},
		},
		{
			id:   "C5",
			desc: "回收意圖 (預期: find_eco_pois, categories=[recycle])",
			userMsg: "大安區哪裡可以丟回收",
			expect:  "find_eco_pois, categories=[recycle]",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "find_eco_pois" {
					return false, fmt.Sprintf("expected find_eco_pois, got %q", name)
				}
				cats := categoriesFromArgs(args)
				if !setEqual(cats, []string{"recycle"}) {
					return false, fmt.Sprintf("categories=%v, want [recycle]", cats)
				}
				return true, fmt.Sprintf("categories=%v", cats)
			},
		},
		{
			id:   "C6",
			desc: "多輪 - 路線後問餐廳 (預期: 只 restaurant, 不沿用區/類別)",
			history: []msg{
				textMsg("user", "從台北101到象山"),
				textMsg("assistant", "已為你規劃從台北101到象山的低碳步行路線, 全程約 1.5km。\n[META:plan_route|origin=台北101|destination=象山]"),
			},
			userMsg: "沿途有什麼環保餐廳",
			expect:  "find_eco_pois, categories=[restaurant], 不沿用 districts",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "find_eco_pois" {
					return false, fmt.Sprintf("expected find_eco_pois, got %q", name)
				}
				cats := categoriesFromArgs(args)
				if !setEqual(cats, []string{"restaurant"}) {
					return false, fmt.Sprintf("categories=%v, want [restaurant]", cats)
				}
				return true, fmt.Sprintf("categories=%v", cats)
			},
		},
		{
			id:   "C7",
			desc: "多輪 districts 不沿用 (預期: T2 沒有 districts 參數)",
			history: []msg{
				textMsg("user", "大安區附近的公園"),
				textMsg("assistant", "為你找到大安區的公園 ...\n[META:find_pois|lat=25.033|lng=121.564|radius=1|categories=park|districts=大安區]"),
			},
			userMsg: "附近有什麼餐廳",
			expect:  "find_eco_pois, categories=[restaurant], **不**帶 districts (沿用會錯)",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "find_eco_pois" {
					return false, fmt.Sprintf("expected find_eco_pois, got %q", name)
				}
				cats := categoriesFromArgs(args)
				_, hasDistricts := args["districts"]
				okCats := setSubset([]string{"restaurant"}, cats) && !setSubset([]string{"hotel"}, cats)
				if !okCats || hasDistricts {
					return false, fmt.Sprintf("categories=%v, has_districts=%v", cats, hasDistricts)
				}
				return true, fmt.Sprintf("categories=%v, no districts", cats)
			},
		},
		{
			id:   "C8",
			desc: "範圍外 (預期: 不呼叫工具, 婉拒)",
			userMsg: "從台北車站到台中火車站",
			expect:  "no tool, 文字婉拒並提雙北",
			checkFn: func(name string, _ map[string]interface{}, text string) (bool, string) {
				if name != "" {
					return false, fmt.Sprintf("expected no tool, got %q", name)
				}
				okText := contains(text, "雙北") || contains(text, "範圍") || contains(text, "服務") || contains(text, "婉拒")
				return okText, fmt.Sprintf("no tool, refusal=%v", okText)
			},
		},
		{
			id:   "C9",
			desc: "順路 ubike (預期: required_categories=[ubike])",
			userMsg: "從台北車站到台北101，順路一個 youbike 站",
			expect:  "plan_eco_route, required_categories=[ubike]",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				req, _ := args["required_categories"].([]interface{})
				if len(req) != 1 || req[0] != "ubike" {
					return false, fmt.Sprintf("required_categories=%v, want [ubike]", req)
				}
				return true, fmt.Sprintf("required_categories=%v", req)
			},
		},
		{
			id:   "C10",
			desc: "中途丟回收 (預期: required_categories=[recycle])",
			userMsg: "從台北市政府走到大安區，中途要丟個回收",
			expect:  "plan_eco_route, required_categories=[recycle]",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				req, _ := args["required_categories"].([]interface{})
				if len(req) != 1 || req[0] != "recycle" {
					return false, fmt.Sprintf("required_categories=%v, want [recycle]", req)
				}
				return true, fmt.Sprintf("required_categories=%v", req)
			},
		},
		{
			id:   "C11",
			desc: "順路咖啡廳 (預期: required_categories=[restaurant])",
			userMsg: "從台北101到象山，順路找個咖啡廳",
			expect:  "plan_eco_route, required_categories=[restaurant]",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				req, _ := args["required_categories"].([]interface{})
				if len(req) != 1 || req[0] != "restaurant" {
					return false, fmt.Sprintf("required_categories=%v, want [restaurant]", req)
				}
				return true, fmt.Sprintf("required_categories=%v", req)
			},
		},
		{
			id:   "C12",
			desc: "無順路語意 — regression: required 不應出現",
			userMsg: "從信義區走到大安區",
			expect:  "plan_eco_route, 沒有 required_categories 或為空",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				req, hasReq := args["required_categories"].([]interface{})
				if hasReq && len(req) > 0 {
					return false, fmt.Sprintf("regression: required_categories=%v 不該出現", req)
				}
				return true, "no required_categories ✓"
			},
		},
		{
			id:   "C13",
			desc: "多類別 (預期: required_categories=[park,recycle])",
			userMsg: "從台北車站到台北101，經過一個公園跟一個回收站",
			expect:  "plan_eco_route, required_categories 包含 park 和 recycle",
			checkFn: func(name string, args map[string]interface{}, _ string) (bool, string) {
				if name != "plan_eco_route" {
					return false, fmt.Sprintf("expected plan_eco_route, got %q", name)
				}
				req, _ := args["required_categories"].([]interface{})
				cats := make([]string, 0, len(req))
				for _, x := range req {
					if s, ok := x.(string); ok {
						cats = append(cats, s)
					}
				}
				okPark := false
				okRecycle := false
				for _, c := range cats {
					if c == "park" {
						okPark = true
					}
					if c == "recycle" {
						okRecycle = true
					}
				}
				if !okPark || !okRecycle {
					return false, fmt.Sprintf("required_categories=%v, want [park, recycle]", cats)
				}
				return true, fmt.Sprintf("required_categories=%v", cats)
			},
		},
	}
}

func main() {
	loadEnvFromFile(".env")
	loadEnvFromFile("Taipei-City-Dashboard-BE/.env")

	apiKey := os.Getenv("TWCC_API_KEY")
	baseURL := os.Getenv("TWCC_API_URL")
	model := os.Getenv("TWCC_MODEL")
	if apiKey == "" || apiKey == "default_your_twcc_api_key_here" {
		fmt.Fprintln(os.Stderr, "ERROR: TWCC_API_KEY not set in env")
		os.Exit(1)
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if model == "" {
		model = defaultModel
	}

	src, err := os.ReadFile("app/controllers/eco_chat.go")
	if err != nil {
		src, err = os.ReadFile("Taipei-City-Dashboard-BE/app/controllers/eco_chat.go")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: cannot read eco_chat.go:", err)
		os.Exit(1)
	}
	prompt, tools, err := extractPromptAndTools(string(src))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR extracting prompt/tools:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	tcs := cases()

	pass, fail := 0, 0
	fmt.Printf("Eco Prompt Smoke Test (%d cases, model=%s, temp=0.01)\n\n", len(tcs), model)
	fmt.Printf("Prompt size: %d chars; Tools: %d\n\n", len(prompt), len(tools))
	for _, c := range tcs {
		fmt.Printf("[%s] %s\n  user: %s\n  expect: %s\n", c.id, c.desc, c.userMsg, c.expect)
		t0 := time.Now()
		toolName, args, text, err := callTWCC(ctx, baseURL, apiKey, model, prompt, tools, c.history, c.userMsg)
		elapsed := time.Since(t0).Round(10 * time.Millisecond)
		if err != nil {
			fail++
			fmt.Printf("  ERROR after %s: %v\n\n", elapsed, err)
			continue
		}
		ok, detail := c.checkFn(toolName, args, text)
		mark := "PASS"
		if !ok {
			mark = "FAIL"
			fail++
		} else {
			pass++
		}
		fmt.Printf("  %s (%s) tool=%q args=%v\n  detail: %s\n", mark, elapsed, toolName, args, detail)
		if text != "" && len(text) < 200 {
			fmt.Printf("  text: %s\n", strings.ReplaceAll(text, "\n", " | "))
		}
		fmt.Println()
	}
	fmt.Printf("=== Result: %d pass / %d fail (total %d) ===\n", pass, fail, len(tcs))
	if fail > 0 {
		os.Exit(1)
	}
}
