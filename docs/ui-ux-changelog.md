# UI/UX refresh — unreleased

## Mobile keyboard and camera follow-up

- Keep the message action sheet open during keyboard-related height changes and
  message-list scrolling. Preserve the touch target until the opening click so
  dismissing the keyboard cannot consume the first menu tap. Width changes still
  dismiss the menu.
- Add a mobile camera button using native rear-camera capture. Reuse the existing
  PNG/JPEG preview, caption, reply, validation and explicit-send flow. Retain the
  separate image picker; native capture depends on browser/device support.

Follow-up: soften element focus to a one-pixel muted blue-grey outline; retain keyboard visibility. Mobile message actions now open a bottom sheet with message preview, direct emoji selection and a large Reply action.

- Render the application shell through stable Lit templates; retain composer identity on connection updates and keep gallery/conversation nodes separate.
- Group consecutive messages from the same sender within five minutes. Retain every timestamp and all reply/reaction/edit/deletion information.
- Replace character-based action symbols with local outline SVG icons. Improve secondary text contrast and keyboard focus visibility while retaining the dark green appearance and full-width conversation.
- Show desktop reply/reaction actions on hover or focus. On mobile use one adjacent action-menu button with a viewport-constrained bottom sheet, preview and direct emoji reactions.
- Add per-device sidebar resizing by dragging or separator arrow keys (Shift for larger steps, Home/End for limits).
- Persist text/reply drafts per chat in this browser. Attachments remain temporary; sending failures preserve the full in-memory draft for manual retry.
- Grow the composer to five lines. Mobile Enter adds a newline; desktop Enter sends and Shift+Enter adds a newline. Ignore composition Enter.
- Simplify in-app Back to always return to the chat selection; remove internal browser history and overlay history entries. Suppress stale search responses.
- Preserve reading anchors when switching back to cached chats or recovering the WhatsApp connection. Keep the connection screen during interruption.
- Remove in-app previews for messages in other chats. Keep unread badges and the existing dedicated mirror-follow option.

- Align the pre-existing service-worker/README version mismatch with package/backend 1.0.0.

Read-state synchronization, first-unread opening and advanced search/media filters are deferred. Backend APIs, deployment settings and the WhatsApp transport are unchanged. See [UI specification](ui-ux-spec.md) for decisions and acceptance criteria.

Executed checks and remaining manual checks: [validation record](ui-ux-validation.md).

## Demo screenshot runner

Add `npm run screenshots` and `scripts/demo-screenshots.sh`: build the frontend,
serve fictional English chats through a loopback-only mock API, validate both
viewports and write the two README screenshots. No real WhatsApp/server data is used.

Add `Dockerfile.screenshots`, `compose.screenshots.yml` and
`scripts/demo-screenshots-docker.sh` to generate the same demo screenshots with
container-provided Node/Chromium/fonts. The runtime has no external network,
reads the built source and writes only temporary browser data and host screenshots.

## Simplified navigation and desktop reaction popovers

Remove desktop Close icons from the reaction picker and reaction overview;
outside click/Escape still dismiss them and return focus. Mobile sheets keep Close.
In-app Back now returns directly to the chat selection from chat, search results
and gallery. Browser/Android Back is no longer intercepted. Remove the UI-history
service and its snapshots/overlay events. Drafts and cached reading positions remain.

Add the existing Wisp icon beside the name in the chat-list header. The brand
links to the GitHub project in a new tab, with accessible labeling and safe
external-link attributes.

## Chat-switch flicker/scroll jump

Cached chat refreshes now preserve the live reading anchor instead of restoring
the position saved when leaving the chat. Keep already loaded history and any
newly received messages when merging the latest server page, including at the
bottom. Recognize cached empty chats. Render a chat switch once, set its initial
position before paint and avoid refocusing the composer when a background refresh
finishes. Ignore deferred scroll callbacks belonging to a different chat.

A Chromium regression holds a refresh response, scrolls the cached chat while it
is pending and samples animation frames after release. Before the fix it reproduced
a jump from 1100px to 600px; after the fix the viewport stays at 1100px with no
loading skeleton during the cached refresh.

## Prepared chat presentation

- Replace the visible skeleton/partial chat with a quiet loading surface (at least
  150 ms). Render at the actual width while hidden, settle the composer and scroll
  position, then reveal without a crossfade. Compose and media actions remain
  disabled during preparation. Search/deep-link context openings follow the same path.
- Prepare cached rows without waiting for the background refresh; keep refresh
  updates anchored. Latest selection and Back cancel older pending reveals.
- Reserve visual-media previews through pending/failed/ready/decoded states.
  Unknown media uses 3:2; show the whole image without cropping. Learn actual
  dimensions in a bounded in-memory cache for later chat visits. No database or
  backend API changes. Video previews use the same stable frames.
- Keep an explicit retry surface after an uncached request fails.

## Intrinsic media frames on first presentation

Expose the retained WhatsApp dimensions on message responses, context/search
results and live messages. Read existing descriptors in one batch without a
database migration or image download. Portrait and landscape previews now use
their original proportions from the first render; small known images are not
enlarged. Only missing metadata uses the existing 3:2 fallback, and an already
visible frame remains stable while media loads.

Fix the media-dimensions regression test fixtures to use distinct WhatsApp IDs;
assert that all four test messages are inserted, avoiding a unique-ID collision
and subsequent foreign-key failure during release checks.

## Camera image upload latency and feedback

Optimize large camera captures locally before preview: maximum 2048-pixel longest
side, JPEG quality 85%, use the result only when it reduces file size. Keep small
photos and ordinary file/paste uploads intact; decoding/encoding failure retains
the original validation path. Show preparation and sending status with a spinner,
block send while preparing or sending, and retain failed drafts for manual retry.
This reduces the bytes sent both to Wisp and onward to WhatsApp; real-world
latency still depends on the mobile uplink and WhatsApp acknowledgement.

## Resolve unknown camera/image frames

Allow an initially unknown media frame to adopt decoded image/video dimensions
once within the same chat visit. Preserve the visible reading anchor or the
latest-message bottom position. Keep known/resolved frames stable across media
status updates; later visits reuse the learned size. This supersedes the earlier
rule that kept an unknown 3:2 fallback for the entire visit. Use proportional
small image previews in the composer as well.

## Confirmed image follow and release version/update information

Follow successful own image sends to the chat bottom, including delayed media,
with a message-context fallback when SSE has not arrived. Preserve loaded
history and guard against chat changes and failed sends.

Embed GitHub release tags into both Docker images and binaries. Show the running
version in Settings and `--version`; development builds remain identifiable.
Add cached, persistent backend release checks every 24 hours with semantic version
comparison, bounded HTTP requests, failure handling and a live app notice.

## Browser Back, PWA images and Windows embedded assets

Restore a bounded browser history: one chat entry above the overview, replaced
on chat switches. Back from the overview leaves Wisp normally. Add a modal image
viewer inside the app with Back/Close/Escape and its own history layer, preserving
the chat. Forward and reload restore chat/image state. This supersedes the earlier
choice to avoid application browser-history entries.

Use platform-independent URL path cleanup for embedded assets. Windows filesystem
cleanup introduced backslashes and failed embedded-FS lookup; the old SPA fallback
to `/index.html` could then produce a `./` redirect loop. Serve fallback requests
through the root on a cloned request instead.

## Image page in the chat pane

Replace the modal image viewer with an image page inside the right chat pane.
Reuse the normal header Back button; Browser Back/Escape also return to the chat.
Keep the desktop sidebar available and the underlying message layout/reading
position intact. Switching chats while viewing an image collapses that image
history layer so the next Back returns to the overview. This supersedes the
modal presentation from the previous follow-up.


Message actions: desktop React/Reply/More controls now mirror around the message
bubble. Mobile replaces per-message buttons with long press and a compact
recent-reaction sheet, with separate full emoji and action views. Text/caption
and image copying work in normal and mirror mode; failed image copying offers
a download. Added real clipboard and touch regressions.
