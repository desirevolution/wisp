# Outgoing messages

The composer snapshots text, reply context and the optional image into a local
outbox before clearing the draft. New typing and attachments are independent
of the previous request. Desktop Enter sends; Shift+Enter inserts a newline.
Mobile Enter still inserts a newline; the Send button starts sending.

## States

- Queued: visible immediately, waiting behind another request in this chat.
- Sending: an HTTP request is running; the composer stays available.
- Failed: show the provider/network error and an explicit Retry send action.
- Confirmed: WhatsApp acknowledged the send, or its live message was observed.
  The pending status disappears. Local image previews remain visible until the
  stored server image has decoded, avoiding a second loading gap. This is not a delivery or read receipt.

Live events and HTTP responses can arrive in either order. Server rows replace
the optimistic row by message ID. No text/content heuristics are used, so two
identical messages remain separate sends. Acknowledgements/context refreshes
in another chat do not move the current reading position or change its draft.

## Retention

IndexedDB stores snapshots including image Blobs on the current device. Text
and reply snapshots additionally have a local-storage fallback. If both stores
are unavailable, pending work remains in memory. Image reload recovery requires
IndexedDB. Restored in-flight requests become uncertain/failed until explicitly
retried; the app does not silently resend after reopening. Confirmed snapshots
remain until the corresponding stored server message replaces them.

A chat-close draft revision also removes older queued/failed snapshots on this
device, including snapshots restored after startup. A request already issued to
WhatsApp cannot be unsent by closing the chat. No new database schema is needed.

## API compatibility

Both `POST /api/chats/{chat}/send` and `send-image` accept the optional header
`X-Wisp-Message-ID`: `3EB0` followed by 18 uppercase hexadecimal digits, using
the same size/prefix as the pinned whatsmeow web message IDs. Legacy requests
without the header retain generated IDs and their existing response contract.
The response remains `{ "ok": true, "id": "..." }`.

Explicit retries reuse the ID. Concurrent requests with the same ID serialize;
an already stored outgoing message in that chat is returned without resending.
Mismatched chat, text, type or reply is rejected. The same ID is also passed to
WhatsApp if the previous result is uncertain. Once WhatsApp acknowledges a send,
a local persistence failure is not reported as a retryable provider failure.
