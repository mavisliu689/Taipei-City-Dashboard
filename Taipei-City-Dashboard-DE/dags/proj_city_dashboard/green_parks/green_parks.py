"""
green_parks DAG

Extract: 並行抓兩源
  - TPE JSON: parks.gov.taipei (25 個 pm_* metadata 欄位)
  - NTPC 河濱 CSV: c3867812... (3 欄: name, longitude, latitude)

Transform: 兩源都對齊 GreenPark 26 欄(25 pm_* + city);缺欄補空字串
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
