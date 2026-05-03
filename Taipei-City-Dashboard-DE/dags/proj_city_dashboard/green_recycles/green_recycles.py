"""
green_recycles DAG

Extract: 兩個 CSV(臺北市 Big5 + 新北市 UTF-8 BOM)
Transform: 統一為 BE green_recycles 13 個業務欄位 + updated_at
  - 新北市來源無經緯度,用 Mapbox forward geocoding 補(需 Airflow Variable: MAPBOX_TOKEN,
    未設則 lat/lng 仍寫 0.0)
Load: replace into DBDashboard.green_recycles
"""
from operators.common_pipeline import CommonDag


# Mapbox 配置:遠低於免費額度上限(100k/月)且新北回收點僅數百筆,單次 DAG 跑完即可。
_MAPBOX_GEOCODE_URL = "https://api.mapbox.com/geocoding/v5/mapbox.places/{addr}.json"
_MAPBOX_MIN_RELEVANCE = 0.7
_MAPBOX_WORKERS = 4
_MAPBOX_TIMEOUT = 10

# 移除地址中半形/全形括號內的補述(如「(永豐公園活動中心旁)」),Mapbox 對這類附註易回 422
# 或 relevance 過低,80 筆抽樣實測可從 81% 提升到 88%。
import re as _re
_PAREN_RE = _re.compile(r"[\(（][^\)）]*[\)）]")


def _clean_address(addr):
    if not addr:
        return ""
    return _PAREN_RE.sub("", addr).strip()


def _geocode_address(addr, token):
    """Mapbox forward geocoding,失敗或 relevance 過低回 (0.0, 0.0)。"""
    import requests
    from urllib.parse import quote

    addr = _clean_address(addr)
    if not addr or not token:
        return 0.0, 0.0
    try:
        url = _MAPBOX_GEOCODE_URL.format(addr=quote(addr, safe=""))
        r = requests.get(
            url,
            params={
                "access_token": token,
                "country": "tw",
                "language": "zh-Hant",
                "limit": 1,
            },
            timeout=_MAPBOX_TIMEOUT,
        )
        r.raise_for_status()
        feats = (r.json() or {}).get("features") or []
        if not feats:
            return 0.0, 0.0
        feat = feats[0]
        if (feat.get("relevance") or 0) < _MAPBOX_MIN_RELEVANCE:
            return 0.0, 0.0
        lng, lat = feat["center"]
        return float(lng), float(lat)
    except Exception as exc:  # noqa: BLE001 - 外部 API,任何錯都不能讓 DAG 整個失敗
        print(f"[mapbox] geocode failed: {addr!r} -> {exc}")
        return 0.0, 0.0


def _geocode_batch(addrs, token):
    """並行 geocoding,回傳 list of (lng, lat),順序與輸入一致。"""
    import concurrent.futures

    with concurrent.futures.ThreadPoolExecutor(max_workers=_MAPBOX_WORKERS) as pool:
        return list(pool.map(lambda a: _geocode_address(a, token), addrs))


def _green_recycles(**kwargs):
    import io
    import pandas as pd
    import requests
    import urllib3
    from airflow.models import Variable
    from sqlalchemy import create_engine
    from utils.get_time import get_tpe_now_time
    from utils.load_stage import (
        save_dataframe_to_postgresql,
        update_lasttime_in_data_to_dataset_info,
    )

    urllib3.disable_warnings()

    # === Config ===
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    dag_infos = kwargs.get("dag_infos", {}) or {}
    dag_id = dag_infos.get("dag_id")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    TPE_CSV_URL = (
        "https://data.taipei/api/dataset/1acf38f3-1509-4cb1-898a-9b1d4f31a3af"
        "/resource/0263f0ce-403a-45ed-a407-c69285b6cad2/download"
    )
    NTPC_CSV_URL = (
        "https://data.ntpc.gov.tw/api/datasets/a381e1f4-86d0-4575-adb4-8d9b6a75e3c4"
        "/csv/file"
    )

    target_cols = [
        "source", "city", "district", "village", "code",
        "name", "address", "phone", "mobile", "open_time", "state",
        "longitude", "latitude",
    ]

    def _decode(content, prefer_encs):
        for enc in prefer_encs:
            try:
                return content.decode(enc)
            except UnicodeDecodeError:
                continue
        return content.decode("utf-8", errors="replace")

    def _str(s):
        return ("" if s is None else str(s)).replace("\n", " ").replace("\r", " ").strip()

    # === Taipei (Big5) ===
    resp = requests.get(TPE_CSV_URL, timeout=120, verify=False)
    resp.raise_for_status()
    text = _decode(resp.content, ("big5", "cp950", "utf-8-sig", "utf-8"))
    tpe_raw = pd.read_csv(io.StringIO(text), header=0)
    tpe_raw = tpe_raw.loc[:, ~tpe_raw.columns.astype(str).str.startswith("Unnamed")]
    # 北市 CSV 欄位順序(按 BE controller fetchRecycleTaipei):
    #   district, name, phone, address, open_time, longitude, latitude
    if tpe_raw.shape[1] < 7:
        raise ValueError("green_recycles 北市 CSV 欄位數不足 7")
    tpe = tpe_raw.iloc[:, :7].copy()
    tpe.columns = ["district", "name", "phone", "address", "open_time", "longitude", "latitude"]
    tpe = tpe[tpe["district"].fillna("").astype(str).str.strip() != ""].copy()
    for c in ["district", "name", "phone", "address", "open_time"]:
        tpe[c] = tpe[c].map(_str)
    tpe["longitude"] = pd.to_numeric(tpe["longitude"], errors="coerce").fillna(0.0)
    tpe["latitude"] = pd.to_numeric(tpe["latitude"], errors="coerce").fillna(0.0)
    tpe["source"] = "taipei"
    tpe["city"] = "臺北市"
    tpe["village"] = ""
    tpe["code"] = ""
    tpe["mobile"] = ""
    tpe["state"] = ""

    # === New Taipei (UTF-8 BOM) ===
    resp = requests.get(NTPC_CSV_URL, timeout=120, verify=False)
    resp.raise_for_status()
    text = _decode(resp.content, ("utf-8-sig", "utf-8", "big5"))
    ntpc_raw = pd.read_csv(io.StringIO(text), header=0)
    ntpc_raw = ntpc_raw.loc[:, ~ntpc_raw.columns.astype(str).str.startswith("Unnamed")]
    # 新北 CSV 欄位順序(按 BE controller fetchRecycleNewTaipei):
    #   seq, district, village, code, name, address(住家), phone, ext, mobile,
    #   recycle_address(實際地點,優先), open_time, state
    if ntpc_raw.shape[1] < 12:
        raise ValueError("green_recycles 新北 CSV 欄位數不足 12")
    ntpc_src = ntpc_raw.iloc[:, :12].copy()
    ntpc_src.columns = [
        "seq", "district", "village", "code", "name", "home_address",
        "phone_main", "phone_ext", "mobile", "recycle_address", "open_time", "state",
    ]
    ntpc_src = ntpc_src[ntpc_src["seq"].fillna("").astype(str).str.strip() != ""].copy()
    # 實際地點優先,空則 fallback 住家
    addr = ntpc_src["recycle_address"].fillna("").astype(str).str.strip()
    fallback_addr = ntpc_src["home_address"].fillna("").astype(str).str.strip()
    addr = addr.where(addr != "", fallback_addr)
    # phone + ext 合併為 "phone#ext"
    phone = ntpc_src["phone_main"].fillna("").astype(str).str.strip()
    ext = ntpc_src["phone_ext"].fillna("").astype(str).str.strip()
    phone = phone.where(ext == "", phone + "#" + ext)
    ntpc = pd.DataFrame({
        "source": "new_taipei",
        "city": "新北市",
        "district": ntpc_src["district"].map(_str),
        "village": ntpc_src["village"].map(_str),
        "code": ntpc_src["code"].map(_str),
        "name": ntpc_src["name"].map(_str),
        "address": addr.map(_str),
        "phone": phone.map(_str),
        "mobile": ntpc_src["mobile"].map(_str),
        "open_time": ntpc_src["open_time"].map(_str),
        "state": ntpc_src["state"].map(_str),
        "longitude": 0.0,
        "latitude": 0.0,
    })

    # 來源無經緯度 → 用 Mapbox forward geocoding 補。
    # token 走 Airflow Variable;未設則直接跳過(維持 0/0,不讓整個 DAG 失敗)。
    mapbox_token = Variable.get("MAPBOX_TOKEN", default_var="")
    if mapbox_token and len(ntpc) > 0:
        addrs = ntpc["address"].tolist()
        print(f"[green_recycles] geocoding {len(addrs)} 新北 addresses via Mapbox...")
        coords = _geocode_batch(addrs, mapbox_token)
        ntpc["longitude"] = [c[0] for c in coords]
        ntpc["latitude"] = [c[1] for c in coords]
        ok = sum(1 for c in coords if c != (0.0, 0.0))
        print(f"[green_recycles] geocoded {ok}/{len(addrs)} ({ok * 100 // max(len(addrs), 1)}%)")
    else:
        print("[green_recycles] MAPBOX_TOKEN not set, 新北 lat/lng 保持 0.0")

    # === Combine ===
    df = pd.concat([tpe[target_cols], ntpc[target_cols]], ignore_index=True)
    df["updated_at"] = get_tpe_now_time()

    # === Load ===
    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine,
        data=df,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )
    update_lasttime_in_data_to_dataset_info(
        engine, airflow_dag_id=dag_id, lasttime_in_data=df["updated_at"].max()
    )


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_recycles")
dag.create_dag(etl_func=_green_recycles)
