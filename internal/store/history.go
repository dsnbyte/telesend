package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// HistoryEntry represents one message delivery record.
type HistoryEntry struct {
	ID              int64           `json:"id"`
	Bot             string          `json:"bot"`
	ChatID          string          `json:"chatId"`
	Recipient       string          `json:"recipient"`
	MessageThreadID *int            `json:"messageThreadId,omitempty"`
	MessageType     string          `json:"messageType"`
	Summary         string          `json:"summary"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	MessageID       *int64          `json:"messageId,omitempty"`
	Status          string          `json:"status"`
	CreatedAt       string          `json:"createdAt"`
}

// HistoryFilter defines query criteria for message history.
type HistoryFilter struct {
	Bot         string
	Recipient   string
	MessageType string
	Limit       int
	Offset      int
}

// History persists and manages message delivery history in SQLite.
type History struct {
	db            *sql.DB
	retentionDays int
}

// NewHistory creates a History manager. If retentionDays <= 0, defaults to 7.
func NewHistory(db *sql.DB, retentionDays int) *History {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	return &History{db: db, retentionDays: retentionDays}
}

// RetentionDays returns the configured retention duration in days.
func (h *History) RetentionDays() int {
	return h.retentionDays
}

// Prune deletes history records older than the retention period relative to referenceTime.
func (h *History) Prune(referenceTime time.Time) (int64, error) {
	cutoff := referenceTime.AddDate(0, 0, -h.retentionDays).Format("2006-01-02T15:04:05.000Z")
	res, err := h.db.Exec("DELETE FROM message_history WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Record inserts a new message delivery record and opportunistically prunes expired records.
func (h *History) Record(entry HistoryEntry) (*HistoryEntry, error) {
	// Opportunistic pruning: clean up expired records on write without background timers.
	_, _ = h.Prune(time.Now().UTC())

	if entry.CreatedAt == "" {
		entry.CreatedAt = now()
	}
	if entry.Status == "" {
		entry.Status = "delivered"
	}
	payloadStr := "{}"
	if len(entry.Payload) > 0 {
		payloadStr = string(entry.Payload)
	}

	res, err := h.db.Exec(
		"INSERT INTO message_history (bot, chat_id, recipient, message_thread_id, message_type, summary, payload, message_id, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		normalizeUsername(entry.Bot),
		entry.ChatID,
		entry.Recipient,
		threadArg(entry.MessageThreadID),
		entry.MessageType,
		entry.Summary,
		payloadStr,
		entry.MessageID,
		entry.Status,
		entry.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	entry.ID = id
	return &entry, nil
}

// List returns delivery history matching the filter, ordered newest first.
// It also triggers opportunistic pruning.
func (h *History) List(filter HistoryFilter) ([]HistoryEntry, error) {
	// Opportunistic pruning on read
	_, _ = h.Prune(time.Now().UTC())

	var conditions []string
	var args []any

	if filter.Bot != "" {
		conditions = append(conditions, "bot = ? COLLATE NOCASE")
		args = append(args, normalizeUsername(filter.Bot))
	}
	if filter.Recipient != "" {
		conditions = append(conditions, "(recipient = ? OR chat_id = ?)")
		args = append(args, filter.Recipient, filter.Recipient)
	}
	if filter.MessageType != "" {
		conditions = append(conditions, "message_type = ?")
		args = append(args, filter.MessageType)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := "SELECT id, bot, chat_id, recipient, message_thread_id, message_type, summary, payload, message_id, status, created_at FROM message_history" + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]HistoryEntry, 0)
	for rows.Next() {
		var e HistoryEntry
		var thread sql.NullInt64
		var msgID sql.NullInt64
		var payloadStr string

		if err := rows.Scan(
			&e.ID,
			&e.Bot,
			&e.ChatID,
			&e.Recipient,
			&thread,
			&e.MessageType,
			&e.Summary,
			&payloadStr,
			&msgID,
			&e.Status,
			&e.CreatedAt,
		); err != nil {
			return nil, err
		}

		if thread.Valid {
			v := int(thread.Int64)
			e.MessageThreadID = &v
		}
		if msgID.Valid {
			v := msgID.Int64
			e.MessageID = &v
		}
		if payloadStr != "" {
			e.Payload = json.RawMessage(payloadStr)
		}

		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Get returns one history entry by ID.
func (h *History) Get(id int64) (*HistoryEntry, error) {
	row := h.db.QueryRow(
		"SELECT id, bot, chat_id, recipient, message_thread_id, message_type, summary, payload, message_id, status, created_at FROM message_history WHERE id = ?",
		id,
	)
	var e HistoryEntry
	var thread sql.NullInt64
	var msgID sql.NullInt64
	var payloadStr string

	err := row.Scan(
		&e.ID,
		&e.Bot,
		&e.ChatID,
		&e.Recipient,
		&thread,
		&e.MessageType,
		&e.Summary,
		&payloadStr,
		&msgID,
		&e.Status,
		&e.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if thread.Valid {
		v := int(thread.Int64)
		e.MessageThreadID = &v
	}
	if msgID.Valid {
		v := msgID.Int64
		e.MessageID = &v
	}
	if payloadStr != "" {
		e.Payload = json.RawMessage(payloadStr)
	}
	return &e, nil
}

// Delete removes a specific history entry by ID.
func (h *History) Delete(id int64) (bool, error) {
	res, err := h.db.Exec("DELETE FROM message_history WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Clear deletes all history records.
func (h *History) Clear() (int64, error) {
	res, err := h.db.Exec("DELETE FROM message_history")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Count returns the total number of history records.
func (h *History) Count() (int, error) {
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM message_history").Scan(&count)
	return count, err
}
