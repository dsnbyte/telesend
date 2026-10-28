package store

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ExportSQL dumps the database schema and data as an SQL script.
func ExportSQL(db *sql.DB) (string, error) {
	var sb strings.Builder

	sb.WriteString("-- Telesend Database Dump\n")
	sb.WriteString(fmt.Sprintf("-- Exported at: %s\n\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString("PRAGMA foreign_keys = OFF;\n")
	sb.WriteString("BEGIN TRANSACTION;\n\n")

	// 1. Get all user tables
	rows, err := db.Query("SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %w", err)
	}
	type tableInfo struct {
		name string
		sql  string
	}
	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.name, &t.sql); err != nil {
			rows.Close()
			return "", err
		}
		tables = append(tables, t)
	}
	if err := rows.Close(); err != nil {
		return "", err
	}

	// 2. Dump schema and data for each table
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("-- Table: %s\n", t.name))
		sb.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", quoteIdent(t.name)))
		sb.WriteString(strings.TrimSpace(t.sql))
		sb.WriteString(";\n")

		// Query rows
		tRows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", quoteIdent(t.name)))
		if err != nil {
			return "", fmt.Errorf("failed to query table %s: %w", t.name, err)
		}

		cols, err := tRows.Columns()
		if err != nil {
			tRows.Close()
			return "", err
		}

		for tRows.Next() {
			values := make([]any, len(cols))
			scanArgs := make([]any, len(cols))
			for i := range values {
				scanArgs[i] = &values[i]
			}
			if err := tRows.Scan(scanArgs...); err != nil {
				tRows.Close()
				return "", err
			}

			sb.WriteString(fmt.Sprintf("INSERT INTO %s VALUES (", quoteIdent(t.name)))
			for i, val := range values {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(formatSQLValue(val))
			}
			sb.WriteString(");\n")
		}
		if err := tRows.Close(); err != nil {
			return "", err
		}
		sb.WriteString("\n")
	}

	// 3. Dump custom indexes
	idxRows, err := db.Query("SELECT sql FROM sqlite_master WHERE type='index' AND sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("failed to list indexes: %w", err)
	}
	var idxCount int
	for idxRows.Next() {
		var idxSQL string
		if err := idxRows.Scan(&idxSQL); err != nil {
			idxRows.Close()
			return "", err
		}
		if strings.TrimSpace(idxSQL) != "" {
			if idxCount == 0 {
				sb.WriteString("-- Indexes\n")
			}
			idxCount++
			sb.WriteString(strings.TrimSpace(idxSQL))
			sb.WriteString(";\n")
		}
	}
	if err := idxRows.Close(); err != nil {
		return "", err
	}
	if idxCount > 0 {
		sb.WriteString("\n")
	}

	sb.WriteString("COMMIT;\n")
	sb.WriteString("PRAGMA foreign_keys = ON;\n")

	return sb.String(), nil
}

// ImportSQL executes an SQL dump against the database, replacing existing data.
func ImportSQL(db *sql.DB, script string) error {
	script = strings.TrimSpace(script)
	if script == "" {
		return fmt.Errorf("SQL script is empty")
	}

	// Execute with foreign keys disabled
	if _, err := db.Exec("PRAGMA foreign_keys = OFF;"); err != nil {
		return err
	}
	defer func() {
		_, _ = db.Exec("PRAGMA foreign_keys = ON;")
	}()

	// Execute the script
	if _, err := db.Exec(script); err != nil {
		return fmt.Errorf("failed to execute SQL import: %w", err)
	}

	// Ensure all standard migrations are up to date
	if err := Migrate(db); err != nil {
		return fmt.Errorf("failed to run migrations after import: %w", err)
	}

	return nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func formatSQLValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'g', -1, 32)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
	case []byte:
		if utf8.Valid(val) {
			return "'" + strings.ReplaceAll(string(val), "'", "''") + "'"
		}
		return "X'" + hex.EncodeToString(val) + "'"
	case time.Time:
		return "'" + val.UTC().Format("2006-01-02T15:04:05.000Z") + "'"
	default:
		s := fmt.Sprintf("%v", val)
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
}
