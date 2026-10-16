import groups from '../../../internal/model/emojis.json' with { type: 'json' };
export const EMOJI_GROUPS = groups;
export const ALL_EMOJIS = EMOJI_GROUPS.flat();

export const EMOJI_ALIASES = new Map([
  ['😀',['grin']],['😃',['smiley']],['😄',['smile']],['😁',['beam']],['😂',['joy','laugh']],
  ['🤣',['rofl']],['😊',['blush']],['😍',['heart-eyes']],['🥰',['love-face']],['😉',['wink']],
  ['🙂',['slight-smile']],['🙃',['upside-down']],['😎',['sunglasses']],['🥳',['party']],['😭',['sob']],
  ['😢',['cry']],['👍',['thumbsup','thumb','+1']],['👎',['thumbsdown','-1']],['👌',['ok']],
  ['✌️',['victory','peace']],['🤞',['fingers-crossed']],['👏',['clap']],['🙏',['pray','thanks']],
  ['💪',['muscle','strong']],['👋',['wave']],['🤝',['handshake']],['👀',['eyes']],['❤️',['heart','love']],
  ['🔥',['fire']],['🎉',['tada','celebrate']],['✅',['check','yes']],['❌',['cross','no']]
]);


export function emojiMatch(input) {
  const pos = input.selectionStart ?? 0;
  const before = input.value.slice(0, pos);
  const match = before.match(/(?:^|\s):([+\-a-z0-9_]*)$/i);
  if (!match || !match[1]) return null;
  const query = match[1].toLowerCase();
  const start = pos - match[1].length - 1;
  const items = ALL_EMOJIS
    .map(emoji => ({ emoji, names: EMOJI_ALIASES.get(emoji) || [] }))
    .filter(item => item.names.some(name => name.includes(query)))
    .slice(0, 6);
  return items.length ? { start, end: pos, items } : null;
}

export function recentEmojis() {
  try {
    return JSON.parse(localStorage.getItem('recentEmojis') || '[]').filter(e => ALL_EMOJIS.includes(e)).slice(0, 12);
  } catch {
    return [];
  }
}

export function rememberEmoji(emoji) {
  const recent = [emoji, ...recentEmojis().filter(item => item !== emoji)].slice(0, 12);
  localStorage.setItem('recentEmojis', JSON.stringify(recent));
}
