/**
 * 起終點名稱 → 座標的對照表。需與 BE 的 placeDictionary 同步。
 * 後端透過 /eco/plan-route 已 geocode 過，但前端要把 polyline 起點延伸到
 * 真實位置時也需要對照。等接外部 geocoding 後可移除。
 */
export const Geocode_dictionary = {
	台北市政府: { lat: 25.0376, lng: 121.5644 },
	市府: { lat: 25.0376, lng: 121.5644 },
	北市府: { lat: 25.0376, lng: 121.5644 },

	新北市政府: { lat: 25.0124, lng: 121.4664 },
	新北市府: { lat: 25.0124, lng: 121.4664 },
	板橋市政府: { lat: 25.0124, lng: 121.4664 },
};
