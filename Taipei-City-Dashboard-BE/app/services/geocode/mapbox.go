// Package geocode 提供批次地址轉經緯度的能力,給 ETL fallback 路徑使用
// (例如新北市資源回收站來源 CSV 不含座標)。token 走 MAPBOX_TOKEN env(fallback
// 到前端共用的 VITE_MAPBOXTOKEN),token 為空時 BatchGeocode 會無聲跳過,
// 不會中斷 fetch 流程。
//
// 與 services/ai/tools/eco/geocode.go 不同:該檔給 LLM 解析地名,有 dictionary
// 與 mock POI 比對;此處只做純地址→座標,不依賴任何字典。
package geocode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"TaipeiCityDashboardBE/logs"
)

// Coord 表示一筆地理座標。失敗或未查到時所有欄位為 0。
type Coord struct {
	Longitude float64
	Latitude  float64
}

const (
	mapboxEndpoint = "https://api.mapbox.com/geocoding/v5/mapbox.places/%s.json"
	// 與 DE green_recycles.py 同樣的閾值,實測樣本下兩者一致表現。
	minRelevance    = 0.7
	defaultWorkers  = 4
	defaultTimeout  = 10 * time.Second
	doubleBeiBBox   = "121.40,24.90,121.75,25.30" // 雙北邊界,sanity check 用
	doubleBeiProx   = "121.5170,25.0478"          // 偏好點(台北車站附近)
)

// 移除地址中半形/全形括號內補述(如「(永豐公園活動中心旁)」),Mapbox 對這類
// 附註易回 422 或 relevance 過低,實測抽樣可從 81% 提升到 88%。
var parenRe = regexp.MustCompile(`[（(][^）)]*[）)]`)

var httpClient = &http.Client{Timeout: defaultTimeout}

// Token 讀取 MAPBOX_TOKEN,若不存在則 fallback 到 VITE_MAPBOXTOKEN(便於開發
// 環境共用前端 token)。回空字串表示未配置 — 呼叫端應跳過 geocoding。
func Token() string {
	if t := os.Getenv("MAPBOX_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("VITE_MAPBOXTOKEN")
}

// CleanAddress 對外暴露主要為測試,正式呼叫由 GeocodeOne 內部使用。
func CleanAddress(addr string) string {
	return strings.TrimSpace(parenRe.ReplaceAllString(addr, ""))
}

// GeocodeOne 將單筆地址轉成 Coord;失敗或 relevance 過低回零值 Coord{}。
// 不回傳 error — 對 ETL 場景而言,個別地址失敗只記 log 不應中斷整批。
func GeocodeOne(addr, token string) Coord {
	addr = CleanAddress(addr)
	if addr == "" || token == "" {
		return Coord{}
	}
	endpoint := fmt.Sprintf(mapboxEndpoint, url.PathEscape(addr))
	q := url.Values{
		"access_token": {token},
		"country":      {"tw"},
		"language":     {"zh-Hant"},
		"limit":        {"1"},
		"bbox":         {doubleBeiBBox},
		"proximity":    {doubleBeiProx},
	}
	resp, err := httpClient.Get(endpoint + "?" + q.Encode())
	if err != nil {
		logs.FWarn("[geocode] http err: %s -> %v", addr, err)
		return Coord{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 422 常見於含特殊字元;其餘狀態碼也只記 log
		return Coord{}
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Features []struct {
			Center    [2]float64 `json:"center"` // [lng, lat]
			Relevance float64    `json:"relevance"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Coord{}
	}
	if len(data.Features) == 0 || data.Features[0].Relevance < minRelevance {
		return Coord{}
	}
	return Coord{
		Longitude: data.Features[0].Center[0],
		Latitude:  data.Features[0].Center[1],
	}
}

// BatchGeocode 並行轉換多筆地址。順序與輸入一致;個別失敗回 Coord{}。
// token 為空則回傳全 Coord{}(不發任何請求)。
func BatchGeocode(addrs []string, token string) []Coord {
	out := make([]Coord, len(addrs))
	if token == "" || len(addrs) == 0 {
		return out
	}
	sem := make(chan struct{}, defaultWorkers)
	var wg sync.WaitGroup
	for i, addr := range addrs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, addr string) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = GeocodeOne(addr, token)
		}(i, addr)
	}
	wg.Wait()
	return out
}
