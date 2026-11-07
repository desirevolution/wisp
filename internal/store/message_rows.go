package store

import "database/sql"

const messageProjection = `SELECT m.id,m.whatsapp_id,m.chat_id,m.sender_id,m.sender_name,m.from_me,
	m.type,m.body,m.has_media,m.media_mime,m.media_filename,m.timestamp,m.created_at,
	md.id,md.status,md.mime_type,md.filename
	FROM messages m LEFT JOIN media md ON md.message_id=m.id`

type messageRow struct {
	id, chatID, kind                         string
	whatsappID, senderID, senderName, body   sql.NullString
	mediaMIME, mediaFilename                 sql.NullString
	mediaID, mediaStatus, mimeType, filename sql.NullString
	fromMe, hasMedia, timestamp, createdAt   int64
}

func (s *Store) queryMessages(query string, args ...any) ([]map[string]any, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var m messageRow
		if err := rows.Scan(&m.id, &m.whatsappID, &m.chatID, &m.senderID, &m.senderName, &m.fromMe,
			&m.kind, &m.body, &m.hasMedia, &m.mediaMIME, &m.mediaFilename, &m.timestamp, &m.createdAt,
			&m.mediaID, &m.mediaStatus, &m.mimeType, &m.filename); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id": m.id, "whatsapp_id": nullable(m.whatsappID), "chat_id": m.chatID,
			"sender_id": nullable(m.senderID), "sender_name": nullable(m.senderName), "from_me": m.fromMe,
			"type": m.kind, "body": nullable(m.body), "has_media": m.hasMedia, "timestamp": m.timestamp,
			"created_at": m.createdAt, "media_mime": nullable(m.mediaMIME), "media_filename": nullable(m.mediaFilename),
			"media_id": nullable(m.mediaID), "media_status": nullable(m.mediaStatus),
			"mime_type": nullable(m.mimeType), "filename": nullable(m.filename),
		})
	}
	return result, rows.Err()
}
