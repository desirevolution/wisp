# Known limitations and troubleshooting

## WhatsApp integration

Wisp uses whatsmeow, an unofficial linked-device implementation. Upstream protocol
changes, revoked sessions and service interruptions can affect it. This is
community software without a delivery or uptime guarantee.

The initial history import is intentionally bounded; older history is requested
on demand. WhatsApp determines which history it supplies. Wisp cannot reconstruct
message/edit versions it never received, or guarantee an exhaustive archive.

Edits received from WhatsApp update the existing text/caption and keep the received
versions. Creating edits in Wisp is not supported. Deletions are marked visibly;
previously stored originals, quotes and cached media may remain available and
can be revealed. Wisp is therefore not a mirror of WhatsApp's deletion retention.

## Push notifications

Push is opt-in per browser/device, needs HTTPS and notification permission, and
is affected by OS/browser policies and network availability. Subscription refresh
runs when the app resumes. Check the permission and reopen Wisp after upgrades.

Every registered device is targeted for a new incoming message. Own messages,
history imports, edits and reactions do not create a new message notification.
The provider TTL is 24 hours, urgency high, and distinct messages use distinct
tags. There is no guaranteed catch-up list: queued provider acceptance, browser
receipt, OS display and sound are separate steps.

Transient failures have bounded retries. The push queue is in memory and does
not survive process restarts; a prolonged outage may lose notification attempts.
A slow provider can delay later messages. Re-enable push if a registration has
expired; inspect the hashed-device log entries to separate devices on a provider.

## Media and sending

Old media can be unavailable in Wisp and official WhatsApp Web alike. A retry may
refresh its path without recovering valid ciphertext. Wisp verifies integrity
and does not bypass hashes. Forwarding a photo creates a new message/descriptor;
it does not repair the original message.

Received media are cached locally, including lazily downloaded history. The media
browser lists files already cached. There is no automatic retention cleanup;
monitor disk usage and back up the cache together with the databases.

Sending supports text, replies, the composer emoji subset and one PNG/JPEG image
up to 10 MiB / 20 megapixels. No arbitrary attachments, GIF/WebP/HEIC uploads,
image-URL downloads or original-quality/document image mode are provided.
Text/image/reply drafts are per-chat browser-memory state and disappear on reload.
After an ambiguous network timeout, verify the chat before manually retrying a
send: provider acceptance can precede an HTTP response failure.

## Clients and security

The embedded UI is English. The desktop and narrow/mobile layouts are covered by
mocked tests; a physical tablet keyboard and push notification delivery need
real-device checks. Keyboard layout uses interactive-widget with a VisualViewport
fallback. Floating keyboards that do not resize either viewport can still cover
content.

The app is intended for one trusted account/installation behind your own access
control. It has no built-in multi-user permissions, login or API tokens. Protect
all routes, including media, SSE and write endpoints, with your reverse proxy.
The supported deployment is Docker on Linux amd64/arm64. Direct binaries,
Windows and a certified iOS/Safari support commitment are outside the 1.0 target.
