# 減碳路徑規劃小助手 — Mock Data

供「減碳路徑規劃 AI 小助手」使用的資料集，已從多個政府開放資料源轉成統一格式。

## 檔案結構

| 檔名 | 筆數 | 說明 |
|---|---|---|
| `parks.json` | 819 | 臺北市公園基本資料 |
| `restaurants.json` | 1000 | 全國環保餐廳（含經緯度） |
| `hotels.json` | 243 | 全國環保標章旅館 |
| `trails.json` | 154 | 臺北市列管登山步道（起點座標） |
| `recycle.json` | 376 | 臺北市限時回收點 + 新北市黃金資收站 |
| `all_points.json` | 2592 | 上述全部合併 |

## 統一 Schema

```ts
interface EcoPoint {
  id: string;              // 唯一識別碼 (category-原始ID)
  name: string;            // 名稱
  category: 'park' | 'restaurant' | 'hotel' | 'trail' | 'recycle';
  lat: number | null;      // 緯度（部份回收站無座標）
  lng: number | null;      // 經度
  address: string;         // 完整地址
  city: string;            // 縣市
  district: string;        // 行政區
  tags: string[];          // 分類標籤（環保等級、步道難度等）
  extra: Record<string, any>;  // 類別專屬欄位
  source: string;          // 資料來源
}
```

## 各類別 `extra` 欄位

- **park**: `area_sqm`, `managing_unit`, `open_start`, `open_end`, `phone`, `facilities`, `transit`, `name_en`
- **restaurant**: `phone`, `mobile`
- **hotel**: `phone`, `village`, `level` (金/銀/銅級環保旅宿)
- **trail**: `length_m`, `duration_min`, `level` (步道分級), `start_point`, `end_point`, `end_lat`, `end_lng`, `barrier_free`, `public_transport`
- **recycle**: `squad`, `phone`, `mobile`, `open_time`, `state`

## 資料來源

| Dataset | 來源 |
|---|---|
| 臺北市公園基本資料 | https://data.gov.tw/dataset/128366 |
| 環保餐廳環境即時通地圖資料 | https://data.gov.tw/dataset/145036 |
| 環保標章旅館環境即時通地圖資料 | https://data.gov.tw/dataset/145035 |
| 臺北市列管登山步道 | https://data.gov.tw/dataset/145689 |
| 臺北市垃圾資源回收限時收受點 | https://data.gov.tw/dataset/132357 |
| 新北市黃金資收站資訊 | https://data.gov.tw/dataset/123351 |

## 重新產生

原始資料下載與轉換腳本位於 `/tmp/eco-mock/build_mock.py`（開發機本機）。如需更新資料，請重新執行該腳本並覆蓋本目錄。
