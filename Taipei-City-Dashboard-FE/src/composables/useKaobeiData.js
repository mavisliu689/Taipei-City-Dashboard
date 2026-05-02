// 黑客松「靠北儀表板」用：把 6 份 Postman fixture 轉成 ComponentConfig，
// 給 contentStore 在 setCurrentDashboardAllContent 偵測 index="kaobei" 時直接灌入。

const CITY = "metrotaipei";

const FIXTURE_LOADERS = {
	parks:      () => import("../dashboardComponent/fixtures/kaobei/parks.json"),
	restaurant: () => import("../dashboardComponent/fixtures/kaobei/restaurant.json"),
	hotel:      () => import("../dashboardComponent/fixtures/kaobei/hotel.json"),
	walkpath:   () => import("../dashboardComponent/fixtures/kaobei/walkpath.json"),
	recycle:    () => import("../dashboardComponent/fixtures/kaobei/recycle.json"),
	ubike:      () => import("../dashboardComponent/fixtures/kaobei/ubike.json"),
};

function topN(map, n) {
	return [...map.entries()]
		.sort((a, b) => b[1] - a[1])
		.slice(0, n)
		.map(([x, y]) => ({ x, y }));
}

function countBy(rows, key) {
	const counts = new Map();
	for (const r of rows) {
		const k = r[key];
		if (!k) continue;
		counts.set(k, (counts.get(k) || 0) + 1);
	}
	return counts;
}

const TRANSFORMS = {
	parks(rows) {
		return [
			{ name: "公園數", data: topN(countBy(rows, "pm_libie"), 10) },
		];
	},
	restaurant(rows) {
		return [
			{ name: "店家數", data: topN(countBy(rows, "city"), 8) },
		];
	},
	hotel(rows) {
		const buckets = { 金級: 0, 銀級: 0, 其他: 0 };
		for (const r of rows) {
			const note = r.note || "";
			if (note.includes("金級")) buckets["金級"] += 1;
			else if (note.includes("銀級")) buckets["銀級"] += 1;
			else buckets["其他"] += 1;
		}
		return [
			{ name: "金級", type: "circle", value: buckets["金級"] },
			{ name: "銀級", type: "circle", value: buckets["銀級"] },
			{ name: "其他", type: "circle", value: buckets["其他"] },
		];
	},
	walkpath(rows) {
		const counts = countBy(rows, "grade");
		return [
			{
				name: "步道分級",
				data: [...counts.entries()].map(([x, y]) => ({ x, y })),
			},
		];
	},
	recycle(rows) {
		return [
			{ name: "回收點數", data: topN(countBy(rows, "district"), 10) },
		];
	},
	ubike(rows) {
		const totals = rows.reduce(
			(acc, r) => {
				if (r.active === false) return acc;
				acc.stations += 1;
				acc.bikes += Number(r.total_quantity) || 0;
				acc.available += Number(r.available_bikes) || 0;
				acc.spots += Number(r.available_spots) || 0;
				return acc;
			},
			{ stations: 0, bikes: 0, available: 0, spots: 0 },
		);
		return [
			{ name: "總站數", data: [totals.stations],   icon: "站" },
			{ name: "總車輛", data: [totals.bikes],      icon: "輛" },
			{ name: "可借",   data: [totals.available],  icon: "輛" },
			{ name: "可還",   data: [totals.spots],      icon: "位" },
		];
	},
};

function configBlueprints() {
	return [
		{
			id: 90001,
			index: "kaobei_parks",
			city: CITY,
			name: "公園綠地",
			source: "Taipei Code Fest API",
			short_desc: "依里別排行 Top 10",
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_parks",
			chart_config: {
				color: ["#7CB342"],
				types: ["BarChart"],
				unit: "座",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_parks",
					type: "circle",
					title: "公園綠地",
					paint: { "circle-color": "#7CB342" },
					property: [
						{ key: "pm_name", name: "名稱" },
						{ key: "pm_location", name: "地址" },
						{ key: "pm_libie", name: "里別" },
					],
					size: "small",
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "pm_libie", yParam: null } },
			history_config: null,
		},
		{
			id: 90002,
			index: "kaobei_restaurant",
			city: CITY,
			name: "環保餐廳",
			source: "Taipei Code Fest API",
			short_desc: "依縣市分布 Top 8",
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_restaurant",
			chart_config: {
				color: ["#FB8C00"],
				types: ["BarChart"],
				unit: "家",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_restaurant",
					type: "circle",
					title: "環保餐廳",
					paint: { "circle-color": "#FB8C00" },
					property: [
						{ key: "name", name: "店名" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
					],
					size: "small",
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "city", yParam: null } },
			history_config: null,
		},
		{
			id: 90003,
			index: "kaobei_hotel",
			city: CITY,
			name: "環保旅宿",
			source: "Taipei Code Fest API",
			short_desc: "依環保標章分級",
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_hotel",
			chart_config: {
				color: ["#FFD700", "#C0C0C0", "#888888"],
				types: ["MapLegend"],
				unit: "家",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_hotel_gold",
					type: "circle",
					title: "金級",
					paint: { "circle-color": "#FFD700" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
				{
					index: "kaobei_hotel_silver",
					type: "circle",
					title: "銀級",
					paint: { "circle-color": "#C0C0C0" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
				{
					index: "kaobei_hotel_other",
					type: "circle",
					title: "其他",
					paint: { "circle-color": "#888888" },
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "note", name: "備註" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byLayer", byParam: null },
			history_config: null,
		},
		{
			id: 90004,
			index: "kaobei_walkpath",
			city: CITY,
			name: "親子步道",
			source: "Taipei Code Fest API",
			short_desc: "依步道分級分布",
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_walkpath",
			chart_config: {
				color: ["#5a9cf8", "#42A5F5", "#1976D2", "#0D47A1"],
				types: ["DonutChart"],
				unit: "條",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_walkpath",
					type: "line",
					title: "親子步道",
					paint: { "line-color": "#5a9cf8" },
					property: [
						{ key: "route", name: "路線" },
						{ key: "district", name: "行政區" },
						{ key: "grade", name: "分級" },
						{ key: "total_length_m", name: "長度(m)" },
					],
					size: null,
					icon: null,
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "grade", yParam: null } },
			history_config: null,
		},
		{
			id: 90005,
			index: "kaobei_recycle",
			city: CITY,
			name: "資源回收點",
			source: "Taipei Code Fest API",
			short_desc: "依行政區排行 Top 10",
			time_from: "static",
			time_to: "static",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_recycle",
			chart_config: {
				color: ["#4CAF50"],
				types: ["BarChart"],
				unit: "點",
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_recycle",
					type: "symbol",
					title: "資源回收點",
					paint: {},
					property: [
						{ key: "name", name: "名稱" },
						{ key: "address", name: "地址" },
						{ key: "phone", name: "電話" },
						{ key: "open_time", name: "開放時間" },
					],
					size: null,
					icon: "triangle_green",
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: { mode: "byParam", byParam: { xParam: "district", yParam: null } },
			history_config: null,
		},
		{
			id: 90006,
			index: "kaobei_ubike",
			city: CITY,
			name: "Ubike 站點",
			source: "Taipei Code Fest API",
			short_desc: "全市即時可借可還統計",
			time_from: "current",
			time_to: "current",
			update_freq: null,
			update_freq_unit: null,
			query_data: "kaobei_ubike",
			chart_config: {
				color: ["#5a9cf8", "#FFFFFF", "#888787"],
				types: ["TextUnitChart"],
				unit: null,
				categories: null,
			},
			chart_data: null,
			map_config: [
				{
					index: "kaobei_ubike",
					type: "symbol",
					title: "Ubike 站點",
					paint: {},
					property: [
						{ key: "name", name: "站名" },
						{ key: "address", name: "地址" },
						{ key: "available_bikes", name: "可借" },
						{ key: "available_spots", name: "可還" },
					],
					size: null,
					icon: "youbike",
					source: "geojson",
					city: CITY,
				},
			],
			map_filter: null,
			history_config: null,
		},
	];
}

const FIXTURE_KEYS = ["parks", "restaurant", "hotel", "walkpath", "recycle", "ubike"];

// Async function 給 contentStore 呼叫，回 6 份 ComponentConfig（chart_data 已填好）
export async function loadKaobeiComponents() {
	const configs = configBlueprints();
	const results = await Promise.allSettled(
		FIXTURE_KEYS.map((k) => FIXTURE_LOADERS[k]()),
	);
	results.forEach((res, idx) => {
		const key = FIXTURE_KEYS[idx];
		if (res.status === "rejected") {
			console.error(`[kaobei] fixture "${key}" load failed`, res.reason);
			configs[idx].chart_data = null;
			return;
		}
		const mod = res.value;
		const payload = mod.default ?? mod;
		const rows = Array.isArray(payload.data) ? payload.data : [];
		try {
			configs[idx].chart_data = TRANSFORMS[key](rows);
		} catch (err) {
			console.error(`[kaobei] transform "${key}" failed`, err);
			configs[idx].chart_data = null;
		}
	});
	return configs;
}

// Sidebar 列表用的 dashboard meta（不含 components，components 會在點進去時才 load）
export const KAOBEI_DASHBOARD_META = {
	index: "kaobei",
	name: "靠北儀表板",
	icon: "eco",
};
