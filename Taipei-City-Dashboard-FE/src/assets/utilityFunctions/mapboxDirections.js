/**
 * Mapbox Directions API client — 取真實步行路線（沿街道）。
 *
 * https://docs.mapbox.com/api/navigation/directions/
 *
 * 用法:
 *   const geojson = await fetchWalkingRoute([[lng,lat],[lng,lat],...])
 *   geojson.geometry.coordinates 就是 polyline。
 */

const BASE = "https://api.mapbox.com/directions/v5/mapbox/walking";

/**
 * @param {[number,number][]} waypoints  - 至少 2 點，每點 [lng, lat]
 * @returns {Promise<{geometry, distance:number, duration:number} | null>}
 */
export async function fetchWalkingRoute(waypoints) {
	if (!waypoints || waypoints.length < 2) return null;
	// Mapbox Directions 上限 25 waypoints
	const trimmed = waypoints.slice(0, 25);
	const coords = trimmed.map(([lng, lat]) => `${lng},${lat}`).join(";");
	const token = import.meta.env.VITE_MAPBOXTOKEN;
	if (!token) {
		console.warn("[mapboxDirections] VITE_MAPBOXTOKEN missing");
		return null;
	}
	const url = `${BASE}/${coords}?geometries=geojson&overview=full&access_token=${token}`;
	try {
		const res = await fetch(url);
		if (!res.ok) {
			console.warn("[mapboxDirections] HTTP", res.status);
			return null;
		}
		const data = await res.json();
		const r = data?.routes?.[0];
		if (!r) return null;
		return {
			geometry: r.geometry,            // GeoJSON LineString
			distance: r.distance,             // metres
			duration: r.duration,             // seconds
		};
	} catch (e) {
		console.warn("[mapboxDirections] fetch failed:", e.message);
		return null;
	}
}
