"""
green_restaurants DAG

Extract: MOENV gis_p_11 (環保餐廳)
Transform: select & rename columns to align with BE green_restaurants table
Load: replace into DBDashboard.green_restaurants
"""
from operators.common_pipeline import CommonDag


def _green_restaurants(**kwargs):
    import pandas as pd
    from sqlalchemy import create_engine
    from utils.extract_stage import get_moenv_json_data
    from utils.get_time import get_tpe_now_time
    from utils.load_stage import (
        save_dataframe_to_postgresql,
        update_lasttime_in_data_to_dataset_info,
    )

    # === Config ===
    ready_data_db_uri = kwargs.get("ready_data_db_uri")
    dag_infos = kwargs.get("dag_infos", {}) or {}
    dag_id = dag_infos.get("dag_id")
    load_behavior = dag_infos.get("load_behavior")
    default_table = dag_infos.get("ready_data_default_table")
    history_table = dag_infos.get("ready_data_history_table")

    # === Extract ===
    raw = get_moenv_json_data("gis_p_11", sort_query="ImportDate desc")
    raw_df = pd.DataFrame(raw)

    # === Transform ===
    target_cols = [
        "restid", "name", "address", "phone", "mobile",
        "latitude", "longitude", "city",
    ]
    for col in target_cols:
        if col not in raw_df.columns:
            raw_df[col] = ""
    df = raw_df[target_cols].copy()
    for c in target_cols:
        df[c] = df[c].fillna("").astype(str).str.replace(r"\s+", " ", regex=True).str.strip()

    # 上游含全國資料,只保留台北市/新北市(容忍臺/台 字形差異)
    target_cities = {"臺北市", "台北市", "新北市"}
    df = df[df["city"].isin(target_cities)].copy()

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


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_restaurants")
dag.create_dag(etl_func=_green_restaurants)
