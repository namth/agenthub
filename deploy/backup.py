#!/usr/bin/env python3
"""Sao lưu SQLite an toàn khi server đang chạy (backup API), giữ 14 bản gần nhất.
Dùng: backup.py <đường dẫn agenthub.db> <thư mục backup>"""
import sqlite3, sys, time
from pathlib import Path

src, out = Path(sys.argv[1]), Path(sys.argv[2])
out.mkdir(parents=True, exist_ok=True)
dst = out / f"agenthub-{time.strftime('%Y%m%d-%H%M%S')}.db"
with sqlite3.connect(src) as s, sqlite3.connect(dst) as d:
    s.backup(d)
for old in sorted(out.glob("agenthub-*.db"))[:-14]:
    old.unlink()
print(dst)
