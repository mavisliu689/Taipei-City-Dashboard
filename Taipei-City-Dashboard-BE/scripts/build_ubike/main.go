// Build ubike.json + 重建 all_points.json (一次性 codegen).
//
// 抓雙北 YouBike2.0 站點資料 (僅 active=1):
//   - 新北市: data.ntpc.gov.tw CSV
//   - 臺北市: tcgbusfs.blob.core.windows.net JSON
//
// 轉成 POI schema 寫到:
//
//	Taipei-City-Dashboard-FE/public/mockData/eco-route/ubike.json
//
// 同時讀取既有 parks/restaurants/hotels/recycle 4 份檔案 + 新產生的 ubike,
// 串成 all_points.json (移除 trail, eco assistant 5 類: park/restaurant/hotel/recycle/ubike).
//
// 用法 (從 repo root):
//
//	go run ./Taipei-City-Dashboard-BE/scripts/build_ubike \
//	    -fe-mock ./Taipei-City-Dashboard-FE/public/mockData/eco-route
//
// CSV 欄位順序與 develop 分支 controllers/green.go 對齊.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ubikeCSVURL        = "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"
	ubikeTaipeiJSONURL = "https://tcgbusfs.blob.core.windows.net/dotapp/youbike/v2/youbike_immediate.json"
)

type poi struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category"`
	Lat      *float64       `json:"lat"`
	Lng      *float64       `json:"lng"`
	Address  string         `json:"address"`
	City     string         `json:"city"`
	District string         `json:"district"`
	Tags     []string       `json:"tags"`
	Extra    map[string]any `json:"extra"`
	Source   string         `json:"source"`
}

type taipeiUbikeRecord struct {
	Sno     string      `json:"sno"`
	Sna     string      `json:"sna"`
	Snaen   string      `json:"snaen"`
	Sarea   string      `json:"sarea"`
	Sareaen string      `json:"sareaen"`
	Ar      string      `json:"ar"`
	Aren    string      `json:"aren"`
	Lat     json.Number `json:"latitude"`
	Lng     json.Number `json:"longitude"`
	Act     string      `json:"act"`
}

func main() {
	feMock := flag.String("fe-mock", "Taipei-City-Dashboard-FE/public/mockData/eco-route",
		"FE mockData/eco-route 目錄 (寫入 ubike.json + all_points.json)")
	flag.Parse()

	if _, err := os.Stat(*feMock); err != nil {
		log.Fatalf("eco-route 目錄不存在: %s (%v)", *feMock, err)
	}

	ntpc, tpe := fetchAll()
	all := append([]poi{}, ntpc...)
	all = append(all, tpe...)
	log.Printf("ubike: NTPC=%d, TPE=%d, total active=%d", len(ntpc), len(tpe), len(all))

	if err := writeJSON(filepath.Join(*feMock, "ubike.json"), all); err != nil {
		log.Fatalf("write ubike.json: %v", err)
	}
	log.Printf("✓ wrote %s (%d stations)", filepath.Join(*feMock, "ubike.json"), len(all))

	rebuildAllPoints(*feMock)
}

func fetchAll() (ntpc, tpe []poi) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		ntpc, err = fetchUbikeNewTaipei()
		if err != nil {
			log.Printf("NTPC fetch failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		tpe, err = fetchUbikeTaipei()
		if err != nil {
			log.Printf("TPE fetch failed: %v", err)
		}
	}()
	wg.Wait()
	if len(ntpc)+len(tpe) == 0 {
		log.Fatal("兩邊 API 都失敗, 中止")
	}
	return ntpc, tpe
}

func fetchUbikeNewTaipei() ([]poi, error) {
	body, err := httpGet(ubikeCSVURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	r := csv.NewReader(body)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]poi, 0, len(records))
	for i, row := range records {
		if i == 0 || len(row) < 18 {
			continue
		}
		sno := parseInt(row[8])
		if sno == 0 || parseInt(row[15]) != 1 {
			continue
		}
		lat := parseFloat(row[12])
		lng := parseFloat(row[13])
		if lat == 0 && lng == 0 {
			continue
		}
		latP, lngP := lat, lng
		out = append(out, poi{
			ID:       fmt.Sprintf("ubike-ntpc-%d", sno),
			Name:     clean(row[2]),
			Category: "ubike",
			Lat:      &latP,
			Lng:      &lngP,
			Address:  clean(row[4]),
			City:     clean(row[0]),
			District: clean(row[3]),
			Tags:     []string{"YouBike", "共享單車", "綠色運輸"},
			Extra: map[string]any{
				"sno":        sno,
				"name_en":    clean(row[5]),
				"area_en":    clean(row[6]),
				"address_en": clean(row[7]),
			},
			Source: "data.ntpc.gov.tw / YouBike2.0 站點 (010e5b15)",
		})
	}
	return out, nil
}

func fetchUbikeTaipei() ([]poi, error) {
	body, err := httpGet(ubikeTaipeiJSONURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var records []taipeiUbikeRecord
	if err := json.NewDecoder(body).Decode(&records); err != nil {
		return nil, err
	}
	out := make([]poi, 0, len(records))
	for _, r := range records {
		sno := parseInt(r.Sno)
		if sno == 0 || parseInt(r.Act) != 1 {
			continue
		}
		lat := parseFloat(r.Lat.String())
		lng := parseFloat(r.Lng.String())
		if lat == 0 && lng == 0 {
			continue
		}
		latP, lngP := lat, lng
		out = append(out, poi{
			ID:       fmt.Sprintf("ubike-tpe-%d", sno),
			Name:     clean(r.Sna),
			Category: "ubike",
			Lat:      &latP,
			Lng:      &lngP,
			Address:  clean(r.Ar),
			City:     "臺北市",
			District: clean(r.Sarea),
			Tags:     []string{"YouBike", "共享單車", "綠色運輸"},
			Extra: map[string]any{
				"sno":        sno,
				"name_en":    clean(r.Snaen),
				"area_en":    clean(r.Sareaen),
				"address_en": clean(r.Aren),
			},
			Source: "tcgbusfs / YouBike2.0 即時 (youbike_immediate)",
		})
	}
	return out, nil
}

func rebuildAllPoints(mockDir string) {
	files := []string{"parks.json", "restaurants.json", "hotels.json", "recycle.json", "ubike.json"}
	merged := make([]poi, 0, 5000)
	for _, f := range files {
		path := filepath.Join(mockDir, f)
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("read %s: %v", path, err)
		}
		var pts []poi
		if err := json.Unmarshal(raw, &pts); err != nil {
			log.Fatalf("parse %s: %v", path, err)
		}
		log.Printf("  + %s: %d", f, len(pts))
		merged = append(merged, pts...)
	}
	out := filepath.Join(mockDir, "all_points.json")
	if err := writeJSON(out, merged); err != nil {
		log.Fatalf("write all_points.json: %v", err)
	}
	log.Printf("✓ rebuilt %s (%d total, 5 categories: park/restaurant/hotel/recycle/ubike)", out, len(merged))
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func httpGet(url string) (rc interface {
	Read(p []byte) (int, error)
	Close() error
}, err error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s -> status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

func clean(s string) string {
	return strings.TrimSpace(strings.Trim(s, "\"'"))
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
