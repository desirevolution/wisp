# 1.0 release scope and short acceptance checklist

## Decisions

- Audience: technically experienced self-hosters.
- Supported deployment: Docker Compose on Linux amd64 and arm64.
- One WhatsApp account per installation; authentication supplied by the proxy.
- Existing 0.17.5 feature scope; no new product features for 1.0.
- Diagnostics: logs only; no persistent diagnostic history or new status UI.
- No required pre-1.0 migration or seven-day soak period.
- GitHub/GHCR publishing and the project license are managed by the owner.

## Stability policy from 1.0

Supported forward updates preserve stored data. If a future stable release needs
a storage migration, implement and test it before publishing that release. Take
a complete backup before updating; rollback requires the matching snapshot.
SQLite tables are internal and are not a public integration contract.

Documented WISP_* deployment options and MQTT payload fields are the integration
surface. Announce/document incompatible changes; do not silently reinterpret
existing values or drop the existing MQTT keys. Consumers should tolerate new
optional fields. Embedded-frontend HTTP/SSE routes remain internal interfaces,
documented for development but not promised to be an immutable public API.

## Automated checks

```sh
docker compose -f compose.checks.yml run --rm --build checks
```

The check container installs dependencies at build time, then runs with no external
network access. `scripts/check.sh` checks version consistency, frontend unit/DOM
suites, Vite production build, Go tests/vet/race, static build and the Chromium
browser suite. Browser tests use fake HTTP/WhatsApp data; they do not connect a
real WhatsApp account. The container needs no credentials or production data.

Current preparation results: [verification.md](verification.md).

## Short deployment acceptance (no minimum waiting period)

Record pass/fail for each item and the Docker image digest used:

- [ ] Build the production image for linux/amd64 and linux/arm64. Run it on each
  architecture (native runner or QEMU) and confirm startup/pairing.
- [ ] Fresh empty data directory: QR pairing completes and initial setup ends.
- [ ] Private/group incoming message, text send, reply, reaction and image send
  with caption work against a real WhatsApp account.
- [ ] A received edit of an own message and a deletion update the existing row.
- [ ] Restart/reconnect retains messages, cached media and the paired session.
- [ ] Update a copied candidate data directory and verify retained content/settings.
- [ ] Restore a complete stopped-container backup with the matching app version.
- [ ] Enable push on two devices and verify new-message deep links; compare logs.
- [ ] Check tablet keyboard, inline timestamps and centered action controls.
- [ ] README/known limitations are visible; initial pairing notice is present.

An unsupported historical media item may remain unavailable as documented. Do
not describe an unexecuted test as passed. Severe reproduced data loss, accidental
sending or credential disclosure must be resolved before calling a build stable.
There is no prescribed seven-day observation period.

## Owner's publishing steps

- [ ] Choose/add the project license.
- [ ] Set up repository and GHCR publishing permissions/workflow.
- [ ] Publish a versioned multi-platform image; record its digest.
- [ ] RC images use only explicit RC tags; do not move stable `latest` to an RC.
- [ ] After acceptance, change package/lock/backend/service-worker/README versions
  together, run all checks and publish `1.0.0` plus the stable tag.
- [ ] Publish the stable release notes and the image-based Compose instructions.

No repository changes, image publication or license selection are performed by
this source package.
