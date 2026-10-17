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
