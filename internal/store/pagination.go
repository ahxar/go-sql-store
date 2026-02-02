package store

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type CursorPage struct {
	Items      interface{} `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}

type OffsetPage struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

type OrderCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

func EncodeCursor(cursor OrderCursor) string {
	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

func DecodeCursor(encoded string) (OrderCursor, error) {
	var cursor OrderCursor
	if encoded == "" {
		return OrderCursor{
			CreatedAt: time.Now(),
			ID:        int64(1<<63 - 1),
		}, nil
	}

	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return cursor, err
	}

	err = json.Unmarshal(data, &cursor)
	return cursor, err
}
