"""
green_walkpaths DAG

Extract: data.taipei CSV (Big5), 22 個欄位
Transform: 中文「是/否」→ bool;數字欄位以 errors='coerce' 解析
Load: replace into DBDashboard.green_walkpaths
"""
from operators.common_pipeline import CommonDag


def _green_walkpaths(**kwargs):
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

    WALKPATHS_CSV_URL = (
        "https://data.taipei/api/dataset/b5726297-d172-4ba7-b5c4-31de38e184e1"
        "/resource/0d1d7db3-efc1-40d1-ad24-5a1a1f88e06b/download"
    )

    # === Extract ===
    resp = requests.get(WALKPATHS_CSV_URL, timeout=120, verify=False)
    resp.raise_for_status()
    csv_text = None
    for enc in ("big5", "cp950", "utf-8-sig", "utf-8"):
        try:
            csv_text = resp.content.decode(enc)
            break
        except UnicodeDecodeError:
            continue
    if csv_text is None:
        csv_text = resp.content.decode("utf-8", errors="replace")
    raw_df = pd.read_csv(io.StringIO(csv_text), header=0)
    # 容忍尾端空欄
    raw_df = raw_df.loc[:, ~raw_df.columns.astype(str).str.startswith("Unnamed")]

    # === Transform ===
    # 上游中文表頭按位置對應到 BE green_walkpaths 22 個欄位(按 BE controller 既有解析順序)
    target_cols = [
        "serial_number", "district", "route", "total_length_m", "one_way_minutes",
        "grade", "start_point", "start_longitude", "start_latitude", "start_is_stairs",
        "end_point", "end_longitude", "end_latitude", "end_is_stairs",
        "has_trail_gate", "wheelchair_friendly", "wheelchair_slope", "wheelchair_length_m",
        "mobile_signal", "has_mobile_toilet", "toilet_location", "accessible_toilet",
    ]
    if raw_df.shape[1] < len(target_cols):
        raise ValueError(
            f"green_walkpaths CSV 欄位數不足: 期待 >= {len(target_cols)},實得 {raw_df.shape[1]}"
        )
    df = raw_df.iloc[:, : len(target_cols)].copy()
    df.columns = target_cols

    # 字串清理
    str_cols = [
        "district", "route", "grade", "start_point", "end_point",
        "wheelchair_slope", "mobile_signal", "toilet_location",
    ]
    for c in str_cols:
        df[c] = df[c].fillna("").astype(str).str.replace(r"\s+", " ", regex=True).str.strip()

    # 整數欄
    for c in ["serial_number", "total_length_m", "one_way_minutes", "wheelchair_length_m"]:
        df[c] = pd.to_numeric(df[c], errors="coerce").fillna(0).astype(int)

    # 浮點座標欄
    for c in ["start_longitude", "start_latitude", "end_longitude", "end_latitude"]:
        df[c] = pd.to_numeric(df[c], errors="coerce").fillna(0.0).astype(float)

    # 中文 是/否 → bool
    bool_cols = [
        "start_is_stairs", "end_is_stairs", "has_trail_gate",
        "wheelchair_friendly", "has_mobile_toilet", "accessible_toilet",
    ]
    for c in bool_cols:
        df[c] = df[c].fillna("").astype(str).str.strip().eq("是")

    # 序號異常列濾掉
    df = df[df["serial_number"] != 0].copy()

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


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_walkpaths")
dag.create_dag(etl_func=_green_walkpaths)
