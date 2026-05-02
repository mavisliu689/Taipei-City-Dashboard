"""
green_ubikes DAG

Extract: 並行抓兩源
  - 台北市 JSON: https://tcgbusfs.blob.core.windows.net/dotapp/youbike/v2/youbike_immediate.json
  - 新北市 CSV: https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file
Transform: normalize 成 11 欄目標 schema(身分 + 位置),用 active=1 過濾停用站
Load: replace into DBDashboard.green_ubikes
"""
from operators.common_pipeline import CommonDag


def _green_ubikes(**kwargs):
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

    TPE_JSON_URL = (
        "https://tcgbusfs.blob.core.windows.net/dotapp/youbike/v2/youbike_immediate.json"
    )
    NTPC_CSV_URL = (
        "https://data.ntpc.gov.tw/api/datasets/010e5b15-3823-4b20-b401-b1cf000550c5/csv/file"
    )

    target_cols = [
        "sno", "name", "name_en", "city", "city_en",
        "area", "area_en", "address", "address_en",
        "latitude", "longitude",
    ]

    def _str(s):
        return ("" if s is None else str(s)).replace("\n", " ").replace("\r", " ").strip()

    # === Taipei JSON ===
    resp = requests.get(TPE_JSON_URL, timeout=120, verify=False)
    resp.raise_for_status()
    tpe_records = resp.json()
    if not isinstance(tpe_records, list):
        raise ValueError("TPE JSON 預期為 list,實際非 list")
    tpe_raw = pd.DataFrame(tpe_records)
    # 過濾 active(JSON act 欄為 string "1")
    if "act" in tpe_raw.columns:
        tpe_raw = tpe_raw[tpe_raw["act"].astype(str).str.strip() == "1"].copy()
    tpe = pd.DataFrame({
        "sno": pd.to_numeric(tpe_raw.get("sno"), errors="coerce").fillna(0).astype("int64"),
        "name": tpe_raw.get("sna", "").map(_str),
        "name_en": tpe_raw.get("snaen", "").map(_str),
        "city": "臺北市",
        "city_en": "Taipei City",
        "area": tpe_raw.get("sarea", "").map(_str),
        "area_en": tpe_raw.get("sareaen", "").map(_str),
        "address": tpe_raw.get("ar", "").map(_str),
        "address_en": tpe_raw.get("aren", "").map(_str),
        "latitude": pd.to_numeric(tpe_raw.get("lat"), errors="coerce").fillna(0.0),
        "longitude": pd.to_numeric(tpe_raw.get("lng"), errors="coerce").fillna(0.0),
    })
    tpe = tpe[tpe["sno"] != 0].copy()

    # === New Taipei CSV ===
    resp = requests.get(NTPC_CSV_URL, timeout=120, verify=False)
    resp.raise_for_status()
    csv_text = None
    for enc in ("utf-8-sig", "utf-8", "big5"):
        try:
            csv_text = resp.content.decode(enc)
            break
        except UnicodeDecodeError:
            continue
    if csv_text is None:
        csv_text = resp.content.decode("utf-8", errors="replace")
    ntpc_raw = pd.read_csv(io.StringIO(csv_text), header=0)
    ntpc_raw = ntpc_raw.loc[:, ~ntpc_raw.columns.astype(str).str.startswith("Unnamed")]
    if ntpc_raw.shape[1] < 18:
        raise ValueError(
            f"green_ubikes 新北 CSV 欄位數不足: 期待 >= 18,實得 {ntpc_raw.shape[1]}"
        )
    # 按 BE controller 既有的位置對應(0:city 1:city_en 2:name 3:area 4:address
    # 5:name_en 6:area_en 7:address_en 8:sno 9:total 10:available 11:mday
    # 12:lat 13:lng 14:spots 15:active 16:yb2 17:eyb)
    ntpc_src = ntpc_raw.iloc[:, :18].copy()
    ntpc_src.columns = [
        "city", "city_en", "name", "area", "address",
        "name_en", "area_en", "address_en",
        "sno", "total", "available", "mday",
        "lat", "lng", "spots", "active", "yb2", "eyb",
    ]
    # 過濾 active=1
    ntpc_src = ntpc_src[
        pd.to_numeric(ntpc_src["active"], errors="coerce").fillna(0).astype(int) == 1
    ].copy()
    ntpc = pd.DataFrame({
        "sno": pd.to_numeric(ntpc_src["sno"], errors="coerce").fillna(0).astype("int64"),
        "name": ntpc_src["name"].map(_str),
        "name_en": ntpc_src["name_en"].map(_str),
        "city": ntpc_src["city"].map(_str),
        "city_en": ntpc_src["city_en"].map(_str),
        "area": ntpc_src["area"].map(_str),
        "area_en": ntpc_src["area_en"].map(_str),
        "address": ntpc_src["address"].map(_str),
        "address_en": ntpc_src["address_en"].map(_str),
        "latitude": pd.to_numeric(ntpc_src["lat"], errors="coerce").fillna(0.0),
        "longitude": pd.to_numeric(ntpc_src["lng"], errors="coerce").fillna(0.0),
    })
    ntpc = ntpc[ntpc["sno"] != 0].copy()

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


dag = CommonDag(proj_folder="proj_city_dashboard", dag_folder="green_ubikes")
dag.create_dag(etl_func=_green_ubikes)
