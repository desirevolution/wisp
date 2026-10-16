export const $ = (selector, root = document) => root.querySelector(selector);

export const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[char]));

export const fmt = timestamp => new Date(timestamp * 1000).toLocaleTimeString([], {
  hour: '2-digit', minute: '2-digit'
});

export const dateFmt = timestamp => new Date(timestamp * 1000).toLocaleDateString([], {
  year: 'numeric', month: 'short', day: 'numeric'
});

export const dayKey = timestamp => {
  const d = new Date(timestamp * 1000);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
};

export function dayLabel(timestamp) {
  const d = new Date(timestamp * 1000);
  const now = new Date();
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (d.toDateString() === now.toDateString()) return 'Today';
  if (d.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return d.getFullYear() === now.getFullYear()
    ? d.toLocaleDateString([], { day: 'numeric', month: 'long' })
    : d.toLocaleDateString([], { day: 'numeric', month: 'long', year: 'numeric' });
}

export function chatStamp(timestamp) {
  if (!timestamp) return '';
  const d = new Date(timestamp * 1000);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return fmt(timestamp);
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (d.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return d.getFullYear() === now.getFullYear()
    ? d.toLocaleDateString([], { day: '2-digit', month: '2-digit' })
    : d.toLocaleDateString([], { day: '2-digit', month: '2-digit', year: '2-digit' });
}

export const mediaLabel = type => ({
  image: '📷 Image', video: '🎥 Video', audio: '🎤 Audio', ptt: '🎤 Voice message',
  document: '📎 Document', sticker: 'Sticker', gif: 'GIF'
}[type] || `[${type}]`);

export const rawId = value => /@(lid|c\.us|g\.us)$/.test(String(value || ''));

export const displayName = (name, id, type) => {
  if (name && !rawId(name)) return name;
  if (type === 'group') return 'Loading group…';
  const digits = String(id || '').split('@')[0].replace(/\D/g, '');
  return digits ? `+${digits}` : 'Loading contact…';
};

export function linkify(text) {
  const source = String(text ?? '');
  const re = /(https?:\/\/[^\s<>"']+|www\.[^\s<>"']+)/gi;
  let out = '', last = 0, match;
  while ((match = re.exec(source))) {
    out += esc(source.slice(last, match.index));
    let shown = match[0], trail = '';
    while (/[),.!?;:]$/.test(shown)) {
      trail = shown.slice(-1) + trail;
      shown = shown.slice(0, -1);
    }
    const href = shown.startsWith('www.') ? `https://${shown}` : shown;
    out += `<a class="msg-link" href="${esc(href)}" target="_blank" rel="noopener noreferrer">${esc(shown)}</a>${esc(trail)}`;
    last = match.index + match[0].length;
  }
  return out + esc(source.slice(last));
}
