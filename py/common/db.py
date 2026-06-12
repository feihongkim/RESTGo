"""
KIS2 MSSQL DB 연결 모듈
config.yaml의 AES-GCM 암호화된 자격증명을 복호화하여 접속
"""
import base64
from contextlib import contextmanager
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
import pymssql
import yaml


_WORKSPACE_CONFIG = "/workspace/config.yaml"
_MAKESQL_CONFIG = "/home/feihong/code/MakeSQL/config.yaml"


def _load_config():
    import os
    path = _WORKSPACE_CONFIG if os.path.exists(_WORKSPACE_CONFIG) else _MAKESQL_CONFIG
    with open(path) as f:
        return yaml.safe_load(f)


def _get_key(fkey: str) -> bytes:
    base64_str = fkey[:-1] + "="
    return base64.b64decode(base64_str)


def _first_decode(enc_text: str, key: bytes) -> str:
    ciphertext = base64.b64decode(enc_text)
    aesgcm = AESGCM(key)
    nonce = ciphertext[:12]
    ct = ciphertext[12:]
    plaintext = aesgcm.decrypt(nonce, ct, None)
    return plaintext.decode()


def _second_decode(encoded: str) -> str:
    if len(encoded) < 14:
        raise ValueError("encoded string too short")
    part1 = encoded[:1]
    part2 = encoded[6:7]
    part3 = encoded[14:]
    original = part1 + part2 + part3
    if original.endswith("_"):
        return original[:-1]
    return original


def decrypt(enc_value: str, key: bytes) -> str:
    first = _first_decode(enc_value, key)
    return _second_decode(first)


def get_connection(server="white", database="KIS2"):
    """MSSQL 연결 반환. 반드시 conn.close() 호출하거나 open_connection() 사용."""
    cfg = _load_config()
    key = _get_key(cfg["FKEY"])
    addr_key = f"MSSQL_ADDR_{server}"
    host = decrypt(cfg.get(addr_key, cfg["MSSQL_ADDR"]), key)
    port = int(decrypt(cfg["MSSQL_PORT"], key))
    user = decrypt(cfg["MSSQL_USER"], key)
    password = decrypt(cfg["MSSQL_PASSWORD"], key)
    return pymssql.connect(
        server=host, port=port, user=user, password=password,
        database=database, charset="utf8",
    )


@contextmanager
def open_connection(server="white", database="KIS2"):
    """Context manager로 DB 연결. with 문 사용 시 자동 close."""
    conn = get_connection(server, database)
    try:
        yield conn
    finally:
        conn.close()
