# Operations

## Deployment and access

Docker Compose on Linux amd64/arm64 is the 1.0 target. The source Compose file
builds the image; `compose.image.yml` uses the owner's published `WISP_IMAGE` tag.
Use one Compose file consistently for all operational commands below (add
`-f compose.image.yml` when running a published image).

Wisp has no built-in authentication. The host port is loopback-only by default.
For other clients, put HTTPS and your own authentication proxy in front of it.
Keep `/api/events` SSE connections streaming and allow up to 10 MiB plus multipart
overhead for image requests. The image-upload operation allows 90 seconds.

A proxy running on the host reaches `127.0.0.1:8080`. A proxy in Docker should
share a network with Wisp and reach `wisp:8080`. In that arrangement remove the
host `ports` mapping and attach your existing proxy network in your deployment's
Compose override. Do not assume a container can reach another container through
its own `127.0.0.1`.

The data directory must be writable by PUID/PGID. Wisp runs without root, with a
read-only container root filesystem and `/tmp` in tmpfs. Do not add an automatic
restart policy based solely on `/health`: it returns 503 during pairing or a
WhatsApp disconnection, which is not necessarily a crashed HTTP process.

## Upgrade

1. Stop Wisp and take a complete backup as below.
2. Keep that backup outside the data directory.
3. For source builds: replace the application source, preserving `.env` and `data/`,
   then run `docker compose up -d --build`.
4. For a published image: change `WISP_IMAGE` to an exact release tag, then run
   `docker compose -f compose.image.yml pull` and
   `docker compose -f compose.image.yml up -d`.
5. Check startup logs and open the app. Confirm a stored chat/media item remains
   available and a new incoming message arrives.
6. Reopen Wisp on each device so the service worker and push subscription refresh.

Never run two instances against the same SQLite/session directory. Do not delete
or reset databases to make an upgrade error disappear.

The storage schema is independent of the app version. This release keeps schema
150, so an existing 0.15–0.17 database of that schema can open unchanged. Earlier
incompatible development schemas have no promised migration. If you choose a
fresh installation, point Compose at a new empty data directory and retain the
old complete directory as backup; expect to pair again. No automatic pre-1.0
conversion or destructive reset is performed.

## Consistent filesystem backup

Run from the directory containing Compose and `data/`. Ensure enough disk space.

```sh
(
  set -e
  umask 077
  mkdir -p backups
  docker compose stop wisp
  tar -czf "backups/wisp-$(date +%Y%m%d-%H%M%S).tar.gz" data
  docker compose start wisp
)
```

If the backup command fails, Wisp remains stopped; resolve the backup failure
before upgrading, or explicitly start the old version again.

The archive must include `wisp.db*`, `whatsmeow.db*` and `media/`. SQLite WAL/SHM
files, if present, belong with their database. The backup contains private
messages, linked-device credentials and push keys; keep it protected.

## Restore or roll back

Stop Wisp. Preserve the current data directory under another name before
extracting a backup; restore the entire snapshot, not one database in isolation.
Use the application version that matches that backup, verify ownership for
PUID/PGID, and start Wisp again. For example, with a chosen backup filename:

```sh
docker compose stop wisp
mv data "data-before-restore-$(date +%Y%m%d-%H%M%S)"
tar -xzf backups/CHOSEN-BACKUP.tar.gz
# Select the image/source version belonging to this snapshot, then start it.
```

Re-linking may still be necessary if WhatsApp revoked the restored device session.
Push subscriptions can need re-enabling when the restored keys differ from those
on a browser. A rollback after a future schema migration requires the matching
full backup; older application versions must not rewrite a newer schema.

## Diagnostics

```sh
docker compose logs --tail=200 wisp
```

The owner controls log retention. The provided Compose file rotates stdout/stderr
at 10 MiB per file, three files. There is no diagnostic table or additional UI.

New push diagnostics use a short hash of the subscription endpoint as `device`.
The same endpoint has the same hash; it is not a hardware identifier and changes
when a subscription is replaced. `provider` contains only its host name.
`message` is a technical WhatsApp ID, not the message body. Endpoint paths,
subscription keys, VAPID secrets and payload text are not included in those logs.
Upstream libraries and older media diagnostics may still include technical IDs
or error details; review logs before sharing them and keep debug logging disabled.

`event=accepted` proves only push-provider acceptance. It does not prove browser
receipt or OS display. `expired` means the dead endpoint is removed. `failed`
shows HTTP status, attempt, duration and whether a network error occurred;
`retry_stopped` explains a provider rate-limit wait beyond the bounded retry window.
