"""
green_parks DAG

Extract: 並行抓三源
  - TPE JSON: parks.gov.taipei (25 個 pm_* metadata 欄位)
  - NTPC 河濱 CSV: c3867812... (3 欄: name, longitude, latitude)
  - NTPC 鄰里 CSV: 5fe3a136... (7 欄: seqno, name, area, address, management,
    localcallservice, areacode);上游無座標,使用 get_addr_xy_parallel geocoding 補

Transform: 三源都對齊 GreenPark 26 欄(25 pm_* + city);缺欄補空字串
Load: replace into DBDashboard.green_parks
"""
from operators.common_pipeline import CommonDag


def _green_parks(**kwargs):
    import io
    import pandas as pd
    import requests
    import urllib3
    from sqlalchemy import create_engine
    from utils.get_time import get_tpe_now_time
    from utils.load_stage import (
        save_dataframe_to_postgresql,
        update_lasttime_in_data_to_dataset_info,
    )
    from utils.transform_address import get_addr_xy_parallel

    urllib3.disable_warnings()

    # === Config ===
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    dag_infos = kwargs.get("dag_infos", {}) or {}
    dag_id = dag_infos.get("dag_id")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    PARKS_TPE_URL = "https://parks.gov.taipei/parks/api/"
    PARKS_NTPC_RIVERSIDE_URL = (
        "https://data.ntpc.gov.tw/api/datasets/c3867812-6188-4b0a-a487-03bb4d93238d/csv"
    )
    PARKS_NTPC_NEIGHBORHOOD_URL = (
        "https://data.ntpc.gov.tw/api/datasets/5fe3a136-29cc-4695-a17e-6636a32c3342/csv"
    )

    # 25 個 pm_* 欄位 + city,跟 BE GreenPark struct 對齊
    pm_cols = [
        "seq_no", "pm_name", "pm_name_eng", "pm_overview",
        "pm_longitude", "pm_latitude",
        "pm_unit", "pm_const_year", "pm_location", "pm_land_public_area",
        "pm_opening_s", "pm_opening_e", "pm_libie", "pm_phone",
        "pm_sports", "pm_recreation", "pm_service", "pm_other",
        "pm_transit", "pm_ecology", "pm_type", "pm_playtype",
        "pm_playarea", "pm_description", "pm_playeq",
    ]
    final_cols = pm_cols + ["city", "updated_at"]

    def _str(s):
        if s is None:
            return ""
        return str(s).replace("\n", " ").replace("\r", " ").strip()

    def _normalize(df, city_value):
        """把任一 source 的 DataFrame 對齊到 26 欄 schema(缺欄補 "")。"""
        out = pd.DataFrame()
        for col in pm_cols:
            if col in df.columns:
                out[col] = df[col].fillna("").astype(str).str.replace(r"\s+", " ", regex=True).str.strip()
            else:
                out[col] = ""
        out["city"] = city_value
        return out

    parts = []

    # === 1. TPE JSON ===
    try:
        resp = requests.get(PARKS_TPE_URL, timeout=120, verify=False)
        resp.raise_for_status()
        tpe_raw = pd.DataFrame(resp.json())
        rename_map = {
            "SeqNo": "seq_no",
            "pm_name": "pm_name",
            "pm_name_eng": "pm_name_eng",
            "pm_overview": "pm_overview",
            "pm_Longitude": "pm_longitude",
            "pm_Latitude": "pm_latitude",
            "pm_unit": "pm_unit",
            "pm_const_year": "pm_const_year",
            "pm_location": "pm_location",
            "pm_LandPublicArea": "pm_land_public_area",
            "pm_opening_s": "pm_opening_s",
            "pm_opening_e": "pm_opening_e",
            "pm_libie": "pm_libie",
            "pm_phone": "pm_phone",
            "pm_sports": "pm_sports",
            "pm_recreation": "pm_recreation",
            "pm_service": "pm_service",
            "pm_other": "pm_other",
            "pm_transit": "pm_transit",
            "pm_ecology": "pm_ecology",
            "pm_type": "pm_type",
            "pm_playtype": "pm_playtype",
            "pm_playarea": "pm_playarea",
            "pm_description": "pm_description",
            "pm_playeq": "pm_playeq",
        }
        tpe_raw = tpe_raw.rename(columns=rename_map)
        parts.append(_normalize(tpe_raw, "臺北市"))
        print(f"[green_parks] TPE: {len(parts[-1])} rows")
    except Exception as e:
        print(f"[green_parks] TPE source failed (will continue): {e}")

    # === 2. NTPC 河濱 CSV ===
    try:
        resp = requests.get(PARKS_NTPC_RIVERSIDE_URL, timeout=120, verify=False)
        resp.raise_for_status()
        text = resp.content.decode("utf-8-sig", errors="replace")
        riv_raw = pd.read_csv(io.StringIO(text), header=0)
        riv_raw = riv_raw.loc[:, ~riv_raw.columns.astype(str).str.startswith("Unnamed")]
        riv_df = pd.DataFrame({
            "pm_name": riv_raw.iloc[:, 0].map(_str) if riv_raw.shape[1] >= 1 else "",
            "pm_longitude": riv_raw.iloc[:, 1].map(_str) if riv_raw.shape[1] >= 2 else "",
            "pm_latitude": riv_raw.iloc[:, 2].map(_str) if riv_raw.shape[1] >= 3 else "",
        })
        riv_df = riv_df[riv_df["pm_name"] != ""].copy()
        parts.append(_normalize(riv_df, "新北市"))
        print(f"[green_parks] NTPC riverside: {len(parts[-1])} rows")
    except Exception as e:
        print(f"[green_parks] NTPC riverside source failed (will continue): {e}")

    # === 3. NTPC 鄰里 CSV (含 geocoding) ===
    try:
        resp = requests.get(PARKS_NTPC_NEIGHBORHOOD_URL, timeout=120, verify=False)
        resp.raise_for_status()
        text = resp.content.decode("utf-8-sig", errors="replace")
        nb_raw = pd.read_csv(io.StringIO(text), header=0)
        nb_raw = nb_raw.loc[:, ~nb_raw.columns.astype(str).str.startswith("Unnamed")]
        if nb_raw.shape[1] < 7:
            raise ValueError(f"鄰里 CSV 欄位數不足:期待 7,實得 {nb_raw.shape[1]}")
        nb_src = nb_raw.iloc[:, :7].copy()
        nb_src.columns = ["seqno", "name", "area", "address", "management", "phone", "areacode"]
        nb_src = nb_src[nb_src["name"].fillna("").astype(str).str.strip() != ""].copy()

        # geocoding:把 address → (lng, lat)。空地址會回 None,後續轉空字串
        addresses = nb_src["address"].fillna("").astype(str)
        try:
            lng_list, lat_list = get_addr_xy_parallel(addresses, sleep_time=0.5)
        except Exception as ge:
            # geocoding 整批失敗就退化:座標留空,name/address 等仍寫進 DB
            print(f"[green_parks] neighborhood geocoding failed, fallback to empty coords: {ge}")
            lng_list = ["" for _ in range(len(addresses))]
            lat_list = ["" for _ in range(len(addresses))]

        nb_df = pd.DataFrame({
            "seq_no": nb_src["seqno"].map(_str),
            "pm_name": nb_src["name"].map(_str),
            "pm_location": nb_src["address"].map(_str),
            "pm_unit": nb_src["management"].map(_str),
            "pm_phone": nb_src["phone"].map(_str),
            "pm_type": nb_src["area"].map(_str),
            "pm_longitude": [_str(v) for v in lng_list],
            "pm_latitude": [_str(v) for v in lat_list],
        })
        parts.append(_normalize(nb_df, "新北市"))
        print(f"[green_parks] NTPC neighborhood: {len(parts[-1])} rows (geocoded)")
    except Exception as e:
        print(f"[green_parks] NTPC neighborhood source failed (will continue): {e}")

    if not parts:
        raise RuntimeError("green_parks: 所有資料源都失敗,終止 ETL")

    # === Concat + Load ===
    df = pd.concat(parts, ignore_index=True)
    df["updated_at"] = get_tpe_now_time()
    ready = df[final_cols].copy()

    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine,
        data=ready,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )
    update_lasttime_in_data_to_dataset_info(
        engine, airflow_dag_id=dag_id, lasttime_in_data=ready["updated_at"].max()
    )


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_parks")
dag.create_dag(etl_func=_green_parks)
