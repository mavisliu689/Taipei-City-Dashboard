import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const FE_ROOT = path.resolve(__dirname, "..");
const FIXTURE_DIR = path.join(FE_ROOT, "src/dashboardComponent/fixtures/kaobei");
const OUT_DIR = path.join(FE_ROOT, "public/mapData");

function num(v) {
	const n = typeof v === "number" ? v : parseFloat(v);
	return Number.isFinite(n) ? n : null;
}

function feature(coords, geomType, properties) {
	return {
		type: "Feature",
		geometry: { type: geomType, coordinates: coords },
		properties,
	};
}

function fc(features) {
	return { type: "FeatureCollection", features };
}

async function loadFixture(name) {
	const buf = await readFile(path.join(FIXTURE_DIR, `${name}.json`), "utf8");
	const parsed = JSON.parse(buf);
	return parsed.data ?? [];
}

async function writeOut(name, geojson) {
	const out = path.join(OUT_DIR, `${name}.geojson`);
	await writeFile(out, JSON.stringify(geojson));
	console.log(`  ✓ ${name.padEnd(24)} ${geojson.features.length} features`);
}

function buildParks(rows) {
	const features = [];
	let skipped = 0;
	for (const r of rows) {
		const lng = num(r.pm_Longitude);
		const lat = num(r.pm_Latitude);
		if (lng === null || lat === null) {
			skipped += 1;
			continue;
		}
		features.push(feature([lng, lat], "Point", { ...r }));
	}
	if (skipped) console.warn(`    parks: skipped ${skipped} rows with bad coords`);
	return fc(features);
}

function buildRestaurant(rows) {
	const features = [];
	let skipped = 0;
	for (const r of rows) {
		const lng = num(r.longitude);
		const lat = num(r.latitude);
		if (lng === null || lat === null) {
			skipped += 1;
			continue;
		}
		features.push(feature([lng, lat], "Point", { ...r }));
	}
	if (skipped) console.warn(`    restaurant: skipped ${skipped} rows with bad coords`);
	return fc(features);
}

function buildHotel(rows) {
	const buckets = { gold: [], silver: [], other: [] };
	let skipped = 0;
	for (const r of rows) {
		const lng = num(r.longitude);
		const lat = num(r.latitude);
		if (lng === null || lat === null) {
			skipped += 1;
			continue;
		}
		const note = r.note || "";
		let bucket = "other";
		if (note.includes("金級")) bucket = "gold";
		else if (note.includes("銀級")) bucket = "silver";
		buckets[bucket].push(feature([lng, lat], "Point", { ...r }));
	}
	if (skipped) console.warn(`    hotel: skipped ${skipped} rows with bad coords`);
	return {
		gold: fc(buckets.gold),
		silver: fc(buckets.silver),
		other: fc(buckets.other),
	};
}

function buildWalkpath(rows) {
	const features = [];
	let skipped = 0;
	for (const r of rows) {
		const sLng = num(r.start_longitude);
		const sLat = num(r.start_latitude);
		const eLng = num(r.end_longitude);
		const eLat = num(r.end_latitude);
		if ([sLng, sLat, eLng, eLat].some((v) => v === null)) {
			skipped += 1;
			continue;
		}
		features.push(
			feature([[sLng, sLat], [eLng, eLat]], "LineString", { ...r }),
		);
	}
	if (skipped) console.warn(`    walkpath: skipped ${skipped} rows with bad coords`);
	return fc(features);
}

function buildRecycle(rows) {
	const features = [];
	let skipped = 0;
	for (const r of rows) {
		const lng = num(r.longitude);
		const lat = num(r.latitude);
		if (lng === null || lat === null) {
			skipped += 1;
			continue;
		}
		features.push(feature([lng, lat], "Point", { ...r }));
	}
	if (skipped) console.warn(`    recycle: skipped ${skipped} rows with bad coords`);
	return fc(features);
}

function buildUbike(rows) {
	const features = [];
	let skipped = 0;
	for (const r of rows) {
		if (r.active === false) continue;
		const lng = num(r.longitude);
		const lat = num(r.latitude);
		if (lng === null || lat === null) {
			skipped += 1;
			continue;
		}
		features.push(feature([lng, lat], "Point", { ...r }));
	}
	if (skipped) console.warn(`    ubike: skipped ${skipped} rows with bad coords`);
	return fc(features);
}

async function main() {
	console.log("[kaobei] reading fixtures from", FIXTURE_DIR);
	const [parks, restaurant, hotel, walkpath, recycle, ubike] = await Promise.all([
		loadFixture("parks"),
		loadFixture("restaurant"),
		loadFixture("hotel"),
		loadFixture("walkpath"),
		loadFixture("recycle"),
		loadFixture("ubike"),
	]);

	console.log("[kaobei] writing geojson to", OUT_DIR);
	await writeOut("kaobei_parks", buildParks(parks));
	await writeOut("kaobei_restaurant", buildRestaurant(restaurant));
	const hotelBuckets = buildHotel(hotel);
	await writeOut("kaobei_hotel_gold", hotelBuckets.gold);
	await writeOut("kaobei_hotel_silver", hotelBuckets.silver);
	await writeOut("kaobei_hotel_other", hotelBuckets.other);
	await writeOut("kaobei_walkpath", buildWalkpath(walkpath));
	await writeOut("kaobei_recycle", buildRecycle(recycle));
	await writeOut("kaobei_ubike", buildUbike(ubike));
	console.log("[kaobei] done.");
}

main().catch((err) => {
	console.error("[kaobei] FAILED:", err);
	process.exit(1);
});
