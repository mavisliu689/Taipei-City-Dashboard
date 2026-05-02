// 雙北 (里 → 區) 解析: park 資料的 district 欄存的是「里」名 (例「長安里」、「八仙」),
// 不是「區」名 — 嚴格 districts=["XX區"] 過濾會永遠 0 筆。本檔提供 (city, 里) → 區
// 反查, 由 scripts/build_li_district 從內政部村里界 CSV codegen。
package eco

import "strings"

type liKey struct {
	City string // "臺北市" / "台北市" / "新北市"
	Li   string // 例「長安里」
}

// liToDistrict is populated by init() in li_to_district_data.go (codegen).
var liToDistrict = map[liKey]string{}

// resolveDistrict 回傳 POI 所屬「區」(三字, 例「中正區」)。
// 若無法判定回 ""。
func resolveDistrict(p POI) string {
	d := strings.TrimSpace(p.District)
	if d == "" {
		return ""
	}
	if strings.HasSuffix(d, "區") {
		return d
	}
	// 1) (city, 里) 完全比對
	if v, ok := liToDistrict[liKey{City: p.City, Li: d}]; ok {
		return v
	}
	// 2) 髒資料: district 沒帶「里」字 (例「八仙」), 補一次再查
	if !strings.HasSuffix(d, "里") {
		if v, ok := liToDistrict[liKey{City: p.City, Li: d + "里"}]; ok {
			return v
		}
	}
	// 3) Fallback: address 含「XX區」 — 解 codegen 表還沒涵蓋的 edge case
	runes := []rune(p.Address)
	for i, r := range runes {
		if r == '區' && i >= 2 {
			return string(runes[i-2 : i+1]) // 例「大安區」
		}
	}
	return ""
}
