"""
green_parks DAG

Extract: parks.gov.taipei JSON API
Transform: rename JSON keys to snake_case columns matching BE GORM model GreenPark
Load: replace into DBDashboard.green_parks (BE 端 GORM AutoMigrate 已預先建表)
"""
from operators.common_pipeline import CommonDag


def _green_parks(**kwargs):
    import requests
    import urllib3
    import pandas as pd
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

    PARKS_API_URL = "https://parks.gov.taipei/parks/api/"

    # === Extract ===
    resp = requests.get(PARKS_API_URL, timeout=120, verify=False)
    resp.raise_for_status()
    raw = resp.json()
    raw_df = pd.DataFrame(raw)

    # === Transform ===
    # 來源 JSON key → BE green_parks 資料表欄位(BE GORM gorm:column 設定一致)
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
    final_cols = list(rename_map.values()) + ["updated_at"]

    df = raw_df.rename(columns=rename_map)
    # 對齊 schema:缺欄補空字串(BE struct 為 string 非指標,沒值就空字串)
    for col in rename_map.values():
        if col not in df.columns:
            df[col] = ""
    df["updated_at"] = get_tpe_now_time()

    ready = df[final_cols].copy()
    # 字串清理:trim + 摺疊空白
    str_cols = [c for c in rename_map.values()]
    for c in str_cols:
        ready[c] = ready[c].fillna("").astype(str).str.replace(r"\s+", " ", regex=True).str.strip()

    # === Load ===
    engine = create_engine(ready_data_db_uri)
    save_dataframe_to_postgresql(
        engine,
        data=ready,
        load_behavior=load_behavior,
        default_table=default_table,
        history_table=history_table,
    )

    # === Update dataset_info.lasttime_in_data ===
    update_lasttime_in_data_to_dataset_info(
        engine, airflow_dag_id=dag_id, lasttime_in_data=ready["updated_at"].max()
    )


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_parks")
dag.create_dag(etl_func=_green_parks)
