package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens an SQLite database at path, ensuring foreign_keys, WAL, and busy_timeout are enabled.
func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	dsn := path
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	dsn = fmt.Sprintf("%s%s_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dsn, separator)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Verify connection can be pinged
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return db, nil
}

// Migrate áp dụng lần lượt các file migrations/NNN_*.sql chưa chạy, mỗi file một transaction,
// ghi lại vào schema_migrations. 001 chỉ gồm CREATE … IF NOT EXISTS / INSERT OR IGNORE nên chạy
// được cả trên DB tạo trước khi có cơ chế này. Gọi nhiều lần an toàn.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		base := strings.TrimPrefix(f, "migrations/")
		version, err := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: tên phải bắt đầu bằng số: %w", base, err)
		}
		var done int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile(f)
		if err != nil {
			return err
		}
		bodyStr := string(body)
		firstLine := strings.TrimRight(strings.SplitN(bodyStr, "\n", 2)[0], "\r")
		if firstLine == "-- agenthub:foreign_keys=off" {
			ctx := context.Background()
			conn, err := db.Conn(ctx)
			if err != nil {
				return fmt.Errorf("migration %s: get conn: %w", base, err)
			}
			runErr := func() error {
				defer func() {
					conn.ExecContext(ctx, "PRAGMA foreign_keys=ON")
					conn.Close()
				}()
				if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
					return fmt.Errorf("migration %s: turn off foreign_keys: %w", base, err)
				}
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil {
					return fmt.Errorf("migration %s: begin tx: %w", base, err)
				}
				if _, err := tx.ExecContext(ctx, bodyStr); err != nil {
					tx.Rollback()
					return fmt.Errorf("migration %s: %w", base, err)
				}
				rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
				if err != nil {
					tx.Rollback()
					return fmt.Errorf("migration %s: pragma foreign_key_check: %w", base, err)
				}
				hasViolation := false
				var violationMsg string
				for rows.Next() {
					hasViolation = true
					var table, parent string
					var rowid, fkid int64
					if err := rows.Scan(&table, &rowid, &parent, &fkid); err == nil {
						violationMsg = fmt.Sprintf("table %s row %d references %s", table, rowid, parent)
					}
				}
				rows.Close()
				if hasViolation {
					tx.Rollback()
					return fmt.Errorf("migration %s: foreign key check failed: %s", base, violationMsg)
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
					tx.Rollback()
					return fmt.Errorf("migration %s: insert schema_migrations: %w", base, err)
				}
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("migration %s: commit tx: %w", base, err)
				}
				return nil
			}()
			if runErr != nil {
				return runErr
			}
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(bodyStr); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", base, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
