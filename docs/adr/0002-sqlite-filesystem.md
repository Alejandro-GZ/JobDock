# ADR 0002: SQLite WAL and filesystem storage

Status: accepted.

The single control-plane instance stores transactional metadata in SQLite WAL and potentially large logs/outputs as files. Database and object-storage behavior remain behind interfaces to permit PostgreSQL and S3-compatible implementations later without introducing those services into the MVP.

