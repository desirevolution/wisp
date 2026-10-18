# UI refresh validation

Validation performed on 17 October 2026 against mocked WhatsApp transport.

## Passed

- Version consistency: package, lockfile, backend version, service-worker cache and README.
- Node unit suite: 28 tests, including persistent text/reply drafts, corrupt/unavailable storage and message-group boundaries.
- All four DOM suites: components/application events, connection states, image composer and timestamps/contrast.
- Vite production build.
- Headless Chromium 153 desktop (1200 × 850): existing reply/reaction/image send and error flows; draft retention across reload; stable textarea identity/focus during connection updates; sidebar keyboard resize and persistence; search result/Back restoration; settings Back; gallery Back; Escape/chat-keyboard/Enter focus; five-line composer; incoming-message reading position/count; disconnect/reconnect reading anchor and draft preservation.
- Headless Chromium mobile emulation (390 × 844, touch): one visible menu action opens a bottom sheet; preview, direct reaction/removal and Reply work; background, Escape and browser Back close the sheet; Enter inserts a newline. A 320 × 480 viewport additionally checks sheet bounds and visible Reply action.
- Mirror mode: send actions/composer hidden.
- Desktop/mobile screenshots inspected internally. Sample fonts in the test environment do not include every emoji.

The browser suite used the project's Playwright package with a separately supplied Chromium executable because the standard browser download was unavailable. No browser/runtime dependency was added to the project.

## Not run / manual checks still required

- Go tests/vet/race/static executable build: Go is not installed in this environment. Backend source is unchanged.
- Real WhatsApp pairing, sending, historical media retrieval, MQTT and push-provider delivery.
- Physical Android keyboard, system Back gesture, orientation changes and installed-PWA lifecycle. Browser emulation is not a substitute for these.
- Docker/multi-architecture builds.

## Existing check repaired

The uploaded source already had package/backend 1.0.0 but an RC service-worker cache and an unversioned README title. These were aligned to 1.0.0 so the existing version check passes and the shell cache changes for the UI update. This is a source revision, not a published release.

## Demo screenshot runner follow-up

`npm run screenshots` / `scripts/demo-screenshots.sh` passed for English desktop
(1440 × 1000) and mobile (430 × 932 CSS pixels, 2× PNG). Both generated README
images were visually inspected. The runner validates the fictional message count,
grouping, composer, absence of browser/HTTP errors and PNG dimensions. Custom
output directory and launching the shell script outside the project were checked.
No production UI, Go backend or dependencies were changed by this follow-up.

## Docker screenshot runner

Dockerfile/Compose configuration and shell syntax were checked. Docker is not
installed in this execution environment, so the image build/container run were
not executed here. The unchanged underlying screenshot runner has passed locally.

## Direct navigation follow-up

The latest behavior supersedes the earlier history-based checks above. Desktop
reaction picker and overview have no visible Close icon; outside click and Escape
were checked, including an overview without action buttons. In-app Back returns
to chat selection from search results, gallery and ordinary chats; browser history
length stays unchanged. The mobile sheet retains Close/Escape/background dismissal.
Draft, focus, scrolling, reaction and image-send checks remain in the Chromium suite.

## Chat-switch regression

Reproduced a cached-refresh jump from 1100px to the old 600px departure position
in a real headless Chromium before applying the fix. The new browser regression
uses two 60-message chats, holds the refresh request, changes the live reading
position, then releases the response and samples animation frames. The viewport
remains at 1100px without a loading skeleton after the fix. The full unit (28),
DOM and desktop/mobile browser suites are run for this revision. This verifies
the reproduced cause, not every device-specific rendering/GPU issue.

## Prepared presentation / stable media follow-up

Based on the newly uploaded `wisp-main(7).zip`.

- Unit suite: 31 passing tests, including fallback/known/learned media dimensions.
- All four DOM suites pass. Application integration waits for the new presentation
  boundary before interacting with the chat.
- Vite production build passes.
- Full Chromium desktop/mobile suite passes, including the previous cached-refresh
  reading-anchor regression.
- Additional real-browser frame sampling confirms a visible loading interval of
  at least 150 ms (frame-sampling tolerance 5 ms), hidden target messages and a
  disabled composer before reveal, and a positioned list at the first visible frame.
- A held image request does not block chat reveal. Decoding a 200 × 800 image leaves
  its fallback frame and following message unchanged. Pending, failed and ready SSE
  media states also retain frame width/height. A later chat visit uses its learned
  1:4 ratio with the full image contained in the frame. The preview was visually inspected.
- Rapid multiple selection reveals only the latest chat. Back cancels pending
  reveals. An uncached fetch failure shows a stable retry surface; retry restores
  the chat and enables composing.
- Returning focus to search during preparation is respected when the chat is ready.
- English demo screenshot smoke test passes with a separate temporary output folder;
  the uploaded README screenshots are preserved.

Intrinsic media sizes are held in bounded session memory, not stored in the database
or browser persistence. The existing backend provides no intrinsic width/height;
no backend or schema change was made. Go/Docker checks and physical Android/WhatsApp
checks were not run in this environment. Chromium emulation does not establish that
all device/GPU-specific flicker is eliminated.

## Mobile keyboard and native camera follow-up

- 31 unit tests, all four DOM suites, Vite build and the complete desktop/mobile
  Chromium suite pass.
- A real touch tap with the composer focused opens the message sheet on the
  first tap. Simulated window resize and message-list scroll leave it open;
  focus leaves the composer and text remains intact.
- The mobile camera button opens a file chooser with rear-camera capture hint.
  Selecting a JPEG creates an image draft without sending or losing the caption.
- Physical Android keyboard animation, native camera launch and photo capture
  still require device verification; Chromium emulation does not supply these.

## Intrinsic media dimensions follow-up

- 31 unit tests, all four DOM suites, production frontend build and the full
  desktop/mobile Chromium suite pass.
- Browser regression checks show pending portrait and landscape frames with
  their supplied 1:2 and 16:9 proportions on initial presentation, independently
  of image downloads. Delayed images still retain their initial frame and do
  not move following messages. Small media bounds are covered by unit tests.
- Added Go regression coverage for wrapped image/video/sticker dimensions,
  invalid/missing/corrupt descriptors, pre-existing retained media and original
  payload fallback, and dimensions on live message context. These Go tests were
  not executed: Go/Docker are unavailable and obtaining the Go toolchain did
  not complete in this environment. Run `go test ./...` before release.
- Database schema and stored content are unchanged; only positive layout sizes
  are exposed, never the original descriptors or media keys.

## Media dimensions test fixture correction

The initial regression fixture omitted `WhatsAppID` on all four messages. SQLite
accepts the first empty unique value, ignores subsequent message inserts through
`INSERT OR IGNORE`, then rejects their orphan context rows. Give each fixture its
message ID as WhatsApp ID and assert successful insertion plus four returned rows.
Replayed the original schema and actual Save SQL in SQLite: reproduced the old
foreign-key failure; the corrected fixture inserts all four messages/contexts and
passes `PRAGMA foreign_key_check`. Go remains unavailable here; rerun
`go test ./internal/whatsapp -run TestStoredMediaDimensionsWorkBeforeDownloadWithoutSchemaChange`
and `go test ./...` in CI. No production schema or runtime change was necessary.

## Camera upload size and send feedback

- 31 unit tests, all four DOM suites, frontend production build and the complete
  desktop/mobile Chromium suite pass.
- Real-browser camera-file regression: a generated 3200x2400 JPEG (458706 bytes)
  becomes 2048x1536 (36921 bytes) before preview/upload. This synthetic gradient
  demonstrates the path, not a promised compression ratio for real photographs.
- A deliberately held image POST shows Sending image and the WhatsApp
  confirmation status with Send disabled; success clears the image/caption.
- Existing upload failure, caption/reply preservation, file/paste, chat-switch
  during send and duplicate-send protection checks continue to pass.
- No backend/schema change. Real mobile capture quality, preparation time and
  end-to-end latency require device verification; no live network timing was
  measured, and provider acknowledgement can still take time.

## Resolve unknown media frames and proportional camera preview

- 31 unit tests, all four DOM suites, production build and full desktop/mobile
  Chromium suite pass.
- Mobile camera preview follows the actual 4:3 image ratio within its bounds.
- A delayed portrait without metadata first uses the fallback, then resolves to
  its decoded 1:4 ratio within the same visit. Bottom/tail position is preserved.
- An image resolving above an older reading position keeps the first visible
  message at the same offset (under 2 px tolerance), with real browser scroll
  anchoring active.
- Pending/failed/ready transitions retain the resolved frame; a later visit uses
  learned dimensions immediately. Known portrait/landscape frames still use
  their original ratios before downloading.
- This intentionally supersedes the earlier entire-visit fallback freeze. No
  backend or database change; physical Android verification remains outstanding.

## Image-send follow, runtime release version and daily update check

Based on the new uploaded `wisp-main(8).zip`.

- All Go packages pass `go test ./...`, including the previously corrected media
  dimension fixture and new release-check tests. `go mod verify` and `go vet ./...`
  pass. Race tests pass for `internal/updates` and `internal/httpapi`.
- New update tests cover numeric semantic ordering (1.10 vs 1.9), prerelease
  current versions, build metadata, development versions, stable-only response
  validation, cancellation, invalid JSON, rate-limit failures, retained known
  releases, persisted daily attempts and restart behavior.
- The compiled CLI returns `wisp v1.2.3` with the injected linker version without
  initializing the server. Source/package/service-worker consistency check passes.
- Workflow YAML parses, shell syntax passes, and the version resolver accepts
  safe release tags while rejecting whitespace, newline and shell substitutions.
  The binary release job now verifies its Linux amd64 `--version` before upload.
- 31 JavaScript unit tests, all four DOM suites, frontend production build and
  the full desktop/mobile Chromium suite pass.
- Browser regressions cover Settings version/release links, the app update
  notice, acknowledged image send without SSE, retained loaded history, late
  image decoding and an upload completed after switching chats. Existing
  incoming-history position behavior continues to pass.
- Docker builds and actual GitHub release publishing were not executed here.
  Update transport is tested with local mock HTTP servers; live GitHub timing
  and real Android/WhatsApp delivery remain device/integration checks.

## Browser Back, image viewer and Windows routing follow-up

- 31 unit tests, all four DOM suites, production frontend build and full
  Chromium desktop/mobile suite pass.
- Browser regression covers a single replaced chat history entry, Back to the
  overview, another Back to a previous external page, Forward, chat reload and
  image-viewer reload. Images stay on the Wisp URL and open no new tab.
- The image viewer closes through Browser Back, Back to chat and Escape while
  preserving the underlying selected chat. Real installed-PWA Android back
  behavior still requires a physical-device check.
- All Go packages pass `go test ./...`; `go vet ./...` passes. Static asset
  regressions assert HTTP 200 without Location on /, JavaScript assets and SPA
  fallback paths, and verify the original request URL is unchanged.
- A Windows amd64 executable cross-compiles successfully as a PE32+ binary.
  This environment does not execute Windows programs. A new Windows CI smoke
  job runs the targeted routing regression natively and gates binary publishing.
- Existing upload follow, update/version, media-frame, reply/reaction and
  position-preservation browser checks continue to pass.
- The HTTP API race suite also passes (`go test -race ./internal/httpapi`).

## Image page in the right chat pane

- 31 unit tests, all four DOM suites, production build, source-version check and
  the complete desktop/mobile Chromium suite pass. No backend changes.
- Image viewing uses a section inside the right conversation grid; no open
  dialog remains. Bounds checks confirm the desktop sidebar is separate and
  interactive. The existing header Back button returns to the chat.
- Browser Back/Forward, image-page reload and Escape continue to pass. Returning
  preserves the underlying chat scroll position. Switching chats from the image
  page collapses the image history layer, so the next Back goes to the overview
  and a further Back reaches the previous external page.
- Physical Android installed-PWA behavior remains a device smoke test.


## Chat lifecycle and mute regression coverage

`npm run test:browser` also runs `tests/browser-chat-controls.spec.mjs`, using
separate desktop and mobile browser contexts and a deterministic API/SSE fixture.
It covers confirmation/cancellation, remote-open chat retention, remote and
offline draft deletion, mute synchronization, unread counts, closed-search
filtering, expiry without reopening, next-message reopening, new contacts,
contact-list retry and modal Escape/Browser Back. Native WhatsApp contact sync,
real delivery, and physical-device keyboard behavior remain manual checks.

Store/API/WhatsApp tests cover state persistence, concurrent close revisions,
history/reaction imports, valid mute durations, contact validation and preserving
UI events. Push and MQTT tests verify both current mute and a queued message's
captured mute decision. Draft unit tests cover revision tombstones across reloads.
