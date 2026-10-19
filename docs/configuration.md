# Configuration

Precedence: CLI > environment > defaults. In Docker set data paths inside `/data`.
The Compose file sets `WISP_DATA_DIR=/data` and `WISP_LISTEN=:8080`.

| CLI | Environment | Default |
| --- | --- | --- |
| `--listen` | `WISP_LISTEN` | `:8080` |
| `--data-dir` | `WISP_DATA_DIR` | `./data` |
| `--db` | `WISP_DB_PATH` | `<data-dir>/wisp.db` |
| `--whatsapp-db` | `WISP_WHATSAPP_DB_PATH` | `<data-dir>/whatsmeow.db` |
| `--media-dir` | `WISP_MEDIA_DIR` | `<data-dir>/media` |
| `--initial-chat-limit` | `WISP_INITIAL_CHAT_LIMIT` | `10` |
| `--initial-messages-per-chat` | `WISP_INITIAL_MESSAGES_PER_CHAT` | `50` |
| `--max-media-mb` | `WISP_MAX_MEDIA_MB` | `100` |
| `--mirror-only` | `WISP_MIRROR_ONLY` | `false` |
| `--mqtt-enabled` | `WISP_MQTT_ENABLED` | `false` |
| `--mqtt-broker` | `WISP_MQTT_BROKER` | empty |
| `--mqtt-username` | `WISP_MQTT_USERNAME` | empty |
| `--mqtt-password` | `WISP_MQTT_PASSWORD` | empty |
| `--mqtt-topic-prefix` | `WISP_MQTT_TOPIC_PREFIX` | `wisp` |

Legacy deployment variables such as `PORT`, `DB_PATH`, `WHATSAPP_DB_PATH`,
`MEDIA_DIR`, `MAX_MEDIA_MB`, `INITIAL_CHAT_LIMIT` and
`INITIAL_MESSAGES_PER_CHAT` remain accepted.

Prefer `WISP_MQTT_PASSWORD` over `--mqtt-password` because command-line secrets
can be visible in shell history and process listings.

## Compose-only variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `PUID`, `PGID` | `1000` | Container user/group; data directory must be writable. |
| `WISP_BIND_ADDRESS` | `127.0.0.1` | Host-side HTTP binding; changing this is a network exposure decision. |
| `WISP_HTTP_PORT` | `8080` | Host-side HTTP port. |
| `TZ` | `Etc/UTC` | Container/server time zone; the UI formats timestamps in the browser's zone. |
| `WISP_IMAGE` | required for image Compose | Exact published image tag. |

`WISP_PUBLIC_DIR` / `--public-dir` is a development UI override, not part of the
supported production deployment.

## MQTT

```text
WISP_MQTT_ENABLED=true
WISP_MQTT_BROKER=tcp://mqtt:1883
WISP_MQTT_USERNAME=wisp
WISP_MQTT_PASSWORD=...
WISP_MQTT_TOPIC_PREFIX=wisp
```

Incoming messages use `<prefix>/message`. Connection status uses `<prefix>/status`.
The message JSON contract is documented in [api-contract.md](api-contract.md).

## Product limits

- Initial import: default 10 chats × 50 messages, older history on demand.
- Incoming media: default 100 MiB; image uploads: 10 MiB and 20 megapixels,
  independently of the incoming-media setting.
- One PNG/JPEG per outgoing image message; text/caption limit 4,000 characters.
- Image/text/reply drafts are in browser memory and disappear on page reload.
- Received reactions may contain any emoji; creation uses the 33-emoji subset (including the quick-reaction defaults).
- There is no automatic chat/media retention policy. Monitor available disk space.

## Build version and daily release check

The application version is a Go linker variable (`wisp/internal/appconfig.Version`),
not a mutable runtime setting. Release workflows pass the GitHub release tag
through `VERSION` to Docker/binary builds. Push builds use `dev-<commit>`; local
build scripts use `VERSION`, an exact local Git tag, or `dev`.

The backend requests the public GitHub latest stable release endpoint for
`desirevolution/wisp` in the background. It uses a 10-second HTTP timeout, a
bounded response and semantic version comparison. It checks when the stored
24-hour interval expires, persists attempts/results in `app_config` without a
schema migration, and retains known release data on transient errors. Fresh
installs check asynchronously on startup. No GitHub token is required. It never
downloads or installs a release automatically.
