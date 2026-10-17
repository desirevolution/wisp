# Wisp UI/UX implementation specification

Status: agreed scope, 17 October 2026. Existing Go API and WhatsApp transport stay compatible.

## Binding decisions

- Main tasks: read/reply and find older messages/media. Desktop mouse/keyboard and Android PWA/browser are equally important.
- Keep the full conversation width. Keep dark appearance, improve text/control contrast. Use consistent local outline SVG icons, accessible labels and tooltips.
- Compact groups on desktop/mobile: consecutive messages by the same sender, direction and day have smaller gaps; display a group sender once. Proposed grouping gap: five minutes.
- Desktop reply/reaction actions appear on hover or keyboard focus without moving bubbles. Mobile has one visible menu button per actionable message, opening a bottom sheet with a message preview, directly selectable emoji and a large Reply action.
- Focused controls get a subtle but visible one-pixel blue-grey outline; active pane gets a subtle indication. Opening a chat focuses the composer on desktop. Escape returns to chat navigation. Mobile opens no keyboard until the input is tapped (an explicit Reply action may focus it).
- Sidebar width is draggable, keyboard adjustable, bounded by available space, and remembered on the device.
- Textarea grows to approximately five lines, then scrolls internally. Desktop Enter sends and Shift+Enter inserts a newline. Mobile Enter inserts a newline; the Send button sends. IME composition must never accidentally send.
- Store text and reply context per chat on the current device across reload/restart. Image attachments and transient errors/sending status are not persisted. Failed sends preserve input and reply/image context for manual retry; no automatic resend.
- While reading older messages, preserve position on incoming messages and show a count/jump button. Incoming messages in other chats only add the existing unread marker (no in-app preview banner). Dedicated mirror follow mode remains available.
- Connection loss pauses interaction and displays the existing connection screen. Preserve loaded conversation position and drafts until reconnected; do not add an offline outbox.
- Opening a search result opens its chat/context. The in-app Back button returns to the chat selection, without restoring the search. Guard against stale search responses.
- The in-app Back button always returns to chat selection. No internal history or popstate handler is used; browser/Android Back retains normal browser behavior. Escape/outside click closes desktop reaction popovers without a Close icon; mobile sheets keep their Close button.

- On chat selection, show a quiet loading surface for at least 150 ms once visible. Build the target list with its real layout dimensions while hidden; set the final reading position before revealing. Disable composing/actions until ready. The most recent selection wins.
- Use cached content without waiting for the server refresh. Apply fresh server data in place afterwards while retaining the live reading anchor.
- Visual media never gates chat reveal. Reserve intrinsic aspect ratio when known; otherwise use a fixed 3:2 preview. Show the complete image with `object-fit: contain`; do not resize that visit's frame after download. Learn intrinsic dimensions in session memory for later visits. Backend dimensions are currently not provided; no backend/schema change is part of this revision.

## Technical implementation choices

Use Lit 3 and the existing store/services; no framework migration. Replace shell string/innerHTML rebuilds with stable Lit templates. Keep message and gallery surfaces as separate stable nodes. Use keyed rendering for messages so updates preserve identity and keyboard focus. Use native popovers with viewport-aware positioning and explicit focus return. Use device-local storage defensively (storage may be blocked/full); keep normal operation available if saving fails. Resize separator supports pointer and keyboard. Navigation uses a direct return-to-selection action and no browser history snapshots.

## Acceptance criteria

1. Connection metadata updates do not replace the textarea or clear an existing draft; reconnect restores the visible reading anchor.
2. Consecutive eligible messages group; sender/day/direction changes and gaps over five minutes start a new group. Every message retains a timestamp.
3. At 390px width there is one action-menu button, not two permanently visible action buttons. Bottom sheet stays inside viewport, includes preview/direct reactions/Reply, is keyboard accessible, and closes on Escape/background click/Close.
4. At desktop width hovering/focusing actions changes no bubble width/position. Focus returns to the trigger after closing a panel. Settings traps focus while open.
5. Reload restores text and reply for each chat. Successful send clears persisted draft; failed send preserves it. Storage failure does not crash the UI.
6. Mobile Enter adds a newline; desktop Enter sends; Shift+Enter adds newline. Composition Enter does not send. Long input stops growing at five lines.
7. Drag/keyboard resize persists and is bounded; resizing the window cannot hide the conversation. Mobile is unaffected.
8. Search -> result -> in-app Back returns to the complete chat selection. Rapid queries cannot display older results. Search errors have an inline retry message.
9. In-app Back returns to the chat selection from both gallery and chat. Menus/settings create no browser history entries. Browser Back is not intercepted.
10. Incoming messages do not move an older reading position; their count appears. Another chat gets an unread badge, without an incoming preview toast.
11. A cached or uncached switch exposes no unpositioned messages. Loading remains visible >=150 ms, input stays disabled until reveal, and stale preparations cannot reveal over the latest choice. Search/deep-link context opening uses the same preparation.
12. A delayed portrait image reveals in a reserved frame without blocking the chat. Loading, failed, ready and decoded states preserve that frame; a later chat visit can use learned dimensions.
13. Existing replies/reactions, edit/deletion history, image sending, mirror mode, PWA and connection flows retain their behavior except the explicit changes above.

## Open / deferred

- Starting at the first unread message was a proposal, not a final decision. Keep current initial latest-message behavior. Preserve positions when returning to a previously opened conversation within the session.
- Definition and multi-device synchronization of read state remain unchanged. No new read receipts.
- Advanced media/search filters were deferred when scope shifted to UI/technical implementation.
- Exact spacing and colors are implementation values to verify visually, not a promise to clone WhatsApp pixel for pixel.

## Validation

Run existing Node unit/DOM tests, frontend production build and Chromium UI checks against mocked transport. Add focused regression coverage for persistent drafts, grouping, mobile Enter, sidebar resizing, direct return-to-selection and focus. Real Android keyboard, Android system Back and real WhatsApp/push delivery require a device smoke test; browser emulation alone does not prove them.
