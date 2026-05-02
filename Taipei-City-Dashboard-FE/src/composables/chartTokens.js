// 「靠北儀表板」5 個組件的圖表配色 token（single source of truth）。
// ApexCharts 不吃 CSS variable（必須是 hex 字串），所以 JS 這份是主來源；
// globalStyles.css :root{} 同名 CSS 變數值與此同步，給 CSS-only 場域（圖例條、tooltip）用。
//
// 改色時：同步改本檔 + globalStyles.css 兩處。

export const CHART_TOKENS = {
	green:     "#81D8D0", // 公園綠地 / 親子步道（綠色階基色）
	purple:    "#BDB2FF", // 環保旅宿
	teal:      "#00B8A9", // 資源回收點
	yellow:    "#FFD166", // Ubike 主黃（圖例 / 預留欄位）
	greyEmpty: "#636E72", // Ubike 「其他」段（topN 之外彙總）
};

// 7 階綠色 ramp（深 → 淺）— 給單色階圖表用（TreemapChart 色塊深淺、BarChart 條色漸層）：
//   index 0 = 最深（小值對應）、index N-1 = 最淺 = base #81D8D0（大值對應）
// 由 blueprint 透過 chart_config.colorRamp 傳給 wrapper，避免 wrapper 直接 import 主 package token
export const GREEN_RAMP = [
	"#0e2622",
	"#173a32",
	"#214f43",
	"#2f6a5b",
	"#447f6e",
	"#5fab94",
	"#81D8D0",
];

// Ubike DonutChart 多段（行政區 top N + 其他）配色：
// 黃→深黃漸層 8 色 + 灰（給最後「其他」段）
export const UBIKE_AREA_PALETTE = [
	"#FFD166",
	"#F4B860",
	"#E6A532",
	"#D4942A",
	"#C28522",
	"#A77019",
	"#8C5C12",
	"#6F4A0E",
	CHART_TOKENS.greyEmpty, // 「其他」段
];
