# Wisp
A lightweight, self-hosted WhatsApp companion with a responsive web UI, messaging, media sharing, push notifications, and MQTT integration.

> Wisp uses an unofficial WhatsApp library [whatsmeow❤️](https://github.com/tulir/whatsmeow) and is not affiliated with WhatsApp/Meta.
> WhatsApp changes may interrupt the connection. Push delivery and recovery of older media are best effort.
> Read [limitations](docs/limitations.md) before linking an account.

## What it does

- Read private/group chats with history, search, media and contact names.
- Send text, replies, PNG/JPEG images and reactions from the shared emoji subset.
- On mobile, use the camera button to take a photo with the native device dialog.
- Show received reactions, edits and deletion markers, including stored context.
- Receive live SSE updates, install the PWA and optionally enable per-device Web Push.
- Publish incoming messages to MQTT
- Optionally run as a read-only mirror.

One WhatsApp account per installation. Authentication belongs to your reverse
proxy; Wisp has no built-in login or user permissions.

## Screenshots

<p align="center">
  <a href="screenshots/wisp-desktop.png?v=2" target="_blank">
    <img src="screenshots/wisp-desktop.png?v=2" width="1200" alt="Desktop">
  </a>
  <br>
  <sub><b>Desktop</b></sub>
</p>

<br>

<p align="center">
  <a href="screenshots/wisp-mobile.png?v=2" target="_blank">
    <img src="screenshots/wisp-mobile.png?v=2" width="400" alt="Mobile">
  </a>
  <br>
  <sub><b>Mobile</b></sub>
</p>

## Docker quick start

From the extracted project directory:

```sh
mkdir -p data
printf 'PUID=%s\nPGID=%s\n' "$(id -u)" "$(id -g)" > .env
docker compose up -d
```

Open `http://localhost:8080` on the Docker host and link Wisp from WhatsApp's
**Linked devices** screen. On a remote Docker host, configure your authenticated
reverse proxy or use an SSH tunnel to reach that loopback address.

The default host binding is **127.0.0.1:8080**. Configure HTTPS and authentication
in your reverse proxy for access from other devices. Proxies in another container
should share a Docker network with Wisp and route to `wisp:8080`; no public host
port is needed in that setup. See [operations](docs/operations.md).

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

New diagnostics are written to the container log. Compose rotates logs at 10 MiB × 3 files.
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

## Generate demo screenshots

With Docker (no local Node or Chromium needed):

```sh
./scripts/demo-screenshots-docker.sh
```

Or run Compose directly (Linux; sets ownership to your user):

```sh
LOCAL_UID=$(id -u) LOCAL_GID=$(id -g) docker compose -f compose.screenshots.yml run --rm --build screenshots
```

The image builds the frontend and includes matching Playwright/Chromium and
browser fonts. Captures are written to the host's `screenshots/` folder and
replace the two README images. Package/image downloads need network access
during the first build; the screenshot container itself runs without external
network access and uses only its internal mock server. After the first build,
you can omit `--build` until code changes. Rebuild after changing the UI or fixtures.

The dedicated Docker start script sets the output file ownership automatically.
Direct Compose uses UID/GID 1000 by default; adjust `LOCAL_UID`/`LOCAL_GID` if
needed. If the Playwright dependency is upgraded, update the matching image tag
in `Dockerfile.screenshots` too.

Without Docker, use the following setup in the project directory:

One-time setup (in the project directory):

```sh
npm ci
npx playwright install chromium
```

Build and capture English desktop/mobile demo screenshots:

```sh
npm run screenshots
# Or use the dedicated start script:
./scripts/demo-screenshots.sh
```

Output: `screenshots/wisp-desktop.png` (1440 × 1000) and
`screenshots/wisp-mobile.png` (860 × 1864, 430 × 932 CSS pixels at 2×).
Existing files with these names are replaced. The README uses these paths.

## About this project

This project was built with AI assistance. I'm a lazy dev.

## License

[MIT](LICENSE).
<<<<<<< Updated upstream
=======

## Version and update checks

Published Docker images and binaries embed the exact GitHub release tag. Check it
in Settings or without starting Wisp:

```sh
./wisp --version
docker run --rm ghcr.io/desirevolution/wisp:latest --version
```

The backend checks the newest stable GitHub release once every 24 hours. Results
and the last attempt are kept in the existing database, including across restarts.
The app shows an update notice with a release link; Settings also show the latest
check time. Checks run in the background and failures do not block chatting.
Updates are installed manually. Development builds display `dev` or `dev-<commit>`
and do not claim to be up to date against a stable release.

For a local versioned build, use `VERSION=v1.2.3 sh build-static.sh`,
`VERSION=v1.2.3 sh scripts/build-binaries.sh`, or Docker's
`--build-arg VERSION=v1.2.3`. Plain `go build` defaults to `dev`; the equivalent
linker option is `-ldflags="-X wisp/internal/appconfig.Version=v1.2.3"`.

After a successful own image send, the chat follows the uploaded image, including
late media layout. Delayed sends do not move another chat's reading position.

## Back navigation and image viewing

Opening a chat adds one browser history entry above the chat overview. Switching
between chats replaces that entry: Browser/Android Back returns to the overview,
and Back again uses normal browser/PWA navigation to leave Wisp. The in-app Back
button also returns to the overview. Browser Forward restores the selected chat.

Chat images and stickers open inside the right-hand chat pane, including in the
installed PWA. The desktop chat list remains available. Use the normal header
Back button, Escape or Browser Back to return to the same chat;
images no longer navigate the PWA to a raw media endpoint or open a separate tab.
The image page and selected chat can also be restored after a reload.


### Close, reopen and mute chats

Use **Chat menu → Close chat…** to hide a conversation across all Wisp devices.
Confirmation is required. Closing marks it as read in Wisp and discards its text,
reply and image drafts on every device. Offline devices discard an old draft
when they next synchronize. Stored messages and media remain subject to the
normal retention settings; nothing is deleted from WhatsApp or marked read there.
A chat already open on another device stays on screen until you leave it.

**New chat** searches stored conversations (including ones labelled **Closed
chat**) and the linked account's locally synchronized WhatsApp contacts. Selecting
a result immediately adds it to the chat list, even before sending anything.
It does not create a WhatsApp group or accept arbitrary phone numbers. Closed
chats and their messages are excluded from the normal search.

Use **Chat menu → Mute chat…** for **8 hours**, **1 week**, or **Indefinitely**.
Mute applies across Wisp devices and suppresses push notifications, MQTT message
publications and in-app auto-follow; internal SSE synchronization continues.
Open chats still show unread counts. A crossed-out bell identifies a muted chat,
and the menu shows its expiry time or offers **Unmute chat**.

Closing and muting are independent. A newly received or successfully sent live
message reopens a closed, unmuted chat. A muted chat stays closed. Reactions,
edits and imported history do not reopen chats. Expiry/unmute alone does not
reopen a chat or replay suppressed notifications; only the next live message
reopens it. Manual reopening keeps its mute setting. WhatsApp's own archive and
mute settings are unchanged.
>>>>>>> Stashed changes
