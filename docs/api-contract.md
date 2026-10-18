# Wisp interfaces — 1.0

The HTTP/SSE endpoints below serve the embedded frontend. They are internal
interfaces rather than a stable public REST API. The documented MQTT message
payload and deployment options are the integration contract for 1.0.

Endpoints used by the embedded frontend:

- `GET /health`
- `GET /api/status`
- `GET|POST /api/config`
- `GET /api/chats`
- `GET /api/chats/:id/messages?limit=&beforeTs=&beforeId=`
- `GET /api/chats/:id/messages/:messageId/context`
- `POST /api/chats/:id/send`
- `POST /api/chats/:id/send-image`
- `GET /api/chats/:id/media`
- `GET /api/chats/:id/links`
- `GET /api/search`
- `GET /api/media/:id`
- `POST /api/media/message/:messageId/retry`
- `GET /api/events` (SSE)
- `POST /api/whatsapp/reconnect`
- `POST /api/whatsapp/logout`
- `GET /api/push/public-key`
- `POST|DELETE /api/push/subscribe`

Runtime user preferences are persisted in `wisp.db`. Deployment paths, limits,
mirror-only mode and MQTT remain CLI/environment policy.

MQTT message payload:

```json
{
  "chatName": "Family",
  "senderName": "Max",
  "isGroup": true,
  "text": "See you soon"
}
```

The gallery endpoints only expose media that is already `ready` and locally
available. Historical media downloads are lazy and may emit media updates over
SSE.

## Replies and reactions

`POST /api/chats/:id/send` accepts `{"text":"Reply text","reply_to":"message-id"}`.
Omit `reply_to` for a normal text message. Targets must belong to that chat.
Message rows and live message events include optional `reply` with `message_id`,
`chat_id`, `sender_id`, `sender_name`, `from_me`, `body` and `type`.
Original protobuf and transport metadata remain private server-side fields.

- `GET /api/chats/:id/messages/:messageId/reactions` returns the current state.
- `POST /api/chats/:id/messages/:messageId/reactions` accepts `{"emoji":"👍"}`.
  Empty string removes your reaction. Only the shared composer subset is accepted
  for creation/replacement. A successful response follows the provider acknowledgement.
- `GET /api/chats/:id/reactions?message_id=a&message_id=b` returns states for up to
  200 targets, used to reconcile after reconnect.

Reaction states contain `chat_id`, `message_id`, `version`, and `reactions`.
Each reaction exposes actor ID/name, ownership, emoji and millisecond timestamp.
Message rows expose `reactions` and `reaction_version`. The dedicated SSE event
`reaction` carries a full state snapshot. Clients ignore lower versions and replace
rather than append state; removals therefore work without a separate client event.
Reactions do not emit ordinary message events, push or MQTT notifications.
Mirror-only mode rejects send operations. Invalid actions return 400, policy
rejection 403, and provider send failures 502. The existing message-context endpoint
is local-only; missing originals return 404.

## Deletions (0.15.2)

Message rows and quote objects expose `deleted` (boolean). Retained body/raw/media
are preserved. `GET /api/chats/:id/deletions` lists known deletion markers for the
chat; the `deletion` SSE event carries `chat_id`, `message_id` and `timestamp`.
Clients mark originals and quote references, reconcile after reconnect, and never
interpret this event as an incoming message. Reply/reaction sending to a deleted
target is rejected. Missing original rows have type `deleted` until recovered.

## Received edits (0.16.0)

Message rows expose `edited`, `edited_at` (milliseconds), `edit_version`,
`edit_history`, and `edit_original_available`. Each historical snapshot contains
`body`, `type`, `timestamp_ms`, and `original` (the first stored baseline).
Raw edit payloads and mention metadata remain private. The latest text is in `body`;
message ID, send timestamp and media identity do not change.

`GET /api/chats/:id/edits?message_id=a&message_id=b` returns up to 200 states.
Each state contains `chat_id`, `message_id`, `body`, `edited_at`, `edit_version`,
`edit_history`, and `original_available`. The SSE event `edit` carries the same state.
An unknown target may have an edit state before its original row arrives; clients
should not invent a new message from that event. Revisions grow when a previously
unseen edit or original baseline is retained, including late older edits.

Clients ignore lower revisions, update cached bodies and previews without reordering
chats, and reconcile loaded messages plus quote target IDs after reconnect. No send,
unread, push, MQTT or follow-new-chat behavior is triggered by an edit event.
Quote `original_edited` flags a received edit later than the parent reply's timestamp;
quote `body` remains the received snapshot. Incoming edits are supported for text and
image/video/document captions; no API for creating edits is introduced.

## Image sending

`POST /api/chats/:id/send-image` accepts multipart `image` (one PNG/JPEG), optional
`caption` and optional `reply_to`. Limits: 10 MiB and 20 megapixels, caption up to
4,000 characters. Actual bytes are validated; filenames/MIME declarations are not
trusted. Returns `{ "ok": true, "id": "..." }` after provider acknowledgement.
Mirror-only mode returns 403; invalid images/actions return 400, excessive request
size 413 and upstream failures 502. The successful image is persisted/cached and
published through the normal own-message/media events.

`GET /api/status` includes the running `version` for technical diagnosis. This is
not proof of WhatsApp connectivity; `/health` returns 503 until WhatsApp is
connected and the database is healthy.

## Media layout dimensions

Message rows (including context/search results where applicable) and live `message`
events expose optional positive `media_width` and `media_height` values from the
original WhatsApp image, video or sticker metadata. Retained descriptors also
support previously stored messages without changing the database schema.
Missing or invalid metadata leaves the fields absent. Raw descriptors and keys
remain private; the API does not fetch image bytes to determine layout.

## Runtime version and release updates

`GET /api/status` exposes the build's `version` and cached `update` state:
`current`, optional `latest`/`url`/`checkedAt`/`error`, and `available`/`comparable`.
`checkedAt` is Unix milliseconds for the latest completed check attempt.
The response is not cached by the browser. A new `update` SSE event carries the
same cached state when the backend check completes. Reading status or opening
Settings does not trigger a GitHub request. Development versions are not
comparable to stable releases. On errors, retain the last known release while
showing that the latest check was unavailable.
