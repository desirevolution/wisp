import { html } from 'lit';

// One local outline vocabulary; no runtime icon downloads.
const paths = {
  close: 'M6 6l12 12M6 18L18 6',
  back: 'M15 18l-6-6 6-6',
  down: 'M12 4v16m-6-6 6 6 6-6',
  reply: 'M9 5l-6 6 6 6M3 11h11a7 7 0 0 1 7 7',
  more: 'M12 5h.01M12 12h.01M12 19h.01',
  image: 'M3 3h18v18H3zM3 17l6-6 4 4 3-6 5 8M8 7h.01',
  send: 'M22 2L9 15M22 2l-8 20-5-7-7-5z',
  smile: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0M8 14s1.5 3 4 3 4-3 4-3M8 9h.01M16 9h.01',
  grid: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
  settings: 'M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1zM16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  paperclip: 'M21 11l-9 9a6 6 0 0 1-8-8L14 2a4 4 0 0 1 6 6L9 19a2 2 0 0 1-3-3L16 6',
};
export const icon = name => html`<svg class="wisp-icon" viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d=${paths[name] || paths.more}></path></svg>`;
