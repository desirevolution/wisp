import { html } from 'lit';

export function replyThumbnail(reply) {
  if (!reply?.media_id || !['image', 'sticker'].includes(reply.type)) return null;
  return html`<img class="quote-thumbnail" src=${`/api/media/${encodeURIComponent(reply.media_id)}`} alt="Quoted image" loading="lazy" @error=${event => { event.currentTarget.hidden = true; }} @load=${event => { event.currentTarget.hidden = false; }}>`;
}
