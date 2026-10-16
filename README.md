# Wisp

A lightweight, self-hosted WhatsApp companion with a responsive web UI, messaging, media sharing, push notifications, and MQTT integration.

> Wisp uses an unofficial WhatsApp library and is not affiliated with WhatsApp/Meta.
> WhatsApp changes may interrupt the connection. Push delivery and recovery of older media are best effort.
> Read [limitations](docs/limitations.md) before linking an account.

## What it does

- Read private/group chats with history, search, media and contact names.
- Send text, replies, PNG/JPEG images and reactions from the shared emoji subset.
- Show received reactions, edits and deletion markers, including stored context.
- Receive live SSE updates, install the PWA and optionally enable per-device Web Push.
- Publish incoming messages to MQTT; optionally run as a read-only mirror.

One WhatsApp account per installation. Authentication belongs to your reverse
proxy; Wisp has no built-in login or user permissions.

## Docker quick start

Docker Compose on Linux **amd64 or arm64** is the supported deployment target.
This candidate includes a source build. Prebuilt GHCR image publication is managed
by the project owner; this archive does not assume a repository or image name.

From the extracted project directory:

```sh
mkdir -p data
printf 'PUID=%s\nPGID=%s\n' "$(id -u)" "$(id -g)" > .env
docker compose up -d --build
```

Open `http://localhost:8080` on the Docker host and link Wisp from WhatsApp's
**Linked devices** screen. On a remote Docker host, configure your authenticated
reverse proxy or use an SSH tunnel to reach that loopback address.

The default host binding is **127.0.0.1:8080**. Configure HTTPS and authentication
in your reverse proxy for access from other devices. Proxies in another container
should share a Docker network with Wisp and route to `wisp:8080`; no public host
port is needed in that setup. See [operations](docs/operations.md).

If you publish an image, users can instead set `WISP_IMAGE` in `.env` to the exact
release image tag and run:

```sh
docker compose -f compose.image.yml pull
docker compose -f compose.image.yml up -d
```

## Configuration

Deployment settings use `WISP_*` environment variables or CLI flags. Sending is
enabled and MQTT disabled by default. Mirror mode and MQTT cannot be changed in
the browser. Runtime preferences such as follow-new-messages remain in SQLite.

See the [configuration reference](docs/configuration.md) for all values, limits,
Compose variables and an MQTT example.

## Updates, backup and restore

Persist the **entire data directory**: `wisp.db`, `whatsmeow.db` and `media/`.
Stop Wisp before taking a filesystem backup so the two SQLite databases, WAL files
and media belong to the same snapshot. Back up before every upgrade.

The 1.0 policy is to retain data across supported forward updates, including any
required schema migrations. Rollback after a storage change requires restoring
the matching full backup. Unsupported schemas are rejected, never silently reset.
There is no promised migration from incompatible pre-1.0 schemas. This candidate
still opens the existing schema 150 used by 0.15–0.17; a reset is not needed for it.

Commands and recovery steps: [operations](docs/operations.md).

## Troubleshooting

```sh
docker compose logs --tail=200 wisp
```

- Connection state: `[whatsapp] event=state_changed`.
- Push: `event=accepted` means the provider accepted the request, not that a device
  displayed it. `expired` removes a dead subscription; `failed` includes the HTTP
  status, attempt and a hashed device identifier.
- Old media can remain unavailable even after a WhatsApp retry; integrity checks
  are never bypassed.
- After an upgrade, reopen Wisp on each device to refresh the PWA/service worker.

New diagnostics are written to the container log only; there is no diagnostic
SQLite history or new status screen. Compose rotates logs at 10 MiB × 3 files.
See [troubleshooting and limitations](docs/limitations.md).

## Development and release checks

With local Node 24, Go 1.26 and a C compiler for race tests:

```sh
npm ci
npx playwright install chromium
npm run check
```

Or run the same checks inside Docker:

```sh
docker compose -f compose.checks.yml run --rm --build checks
```

The checks cover unit and DOM tests, the production frontend build, Go tests/vet,
race checks, a static executable build, and real Chromium tests with mocked
WhatsApp transport. They do not prove real WhatsApp, mobile keyboard or push
provider delivery. Use the short [release checklist](docs/release.md).

Direct binaries and Windows are experimental developer paths for now, outside
the 1.0 support target. Their build script remains available.

## About this project

This project was built with AI assistance. I'm a lazy dev.

## License

[MIT](LICENSE).
