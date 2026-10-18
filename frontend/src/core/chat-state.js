export const chatMuted = (state, now = Date.now()) => state?.mute_until === -1 || Number(state?.mute_until) * 1000 > now;
export function muteLabel(state, now = Date.now()) {
 if (!chatMuted(state, now)) return '';
 return state.mute_until === -1 ? 'Muted indefinitely' : `Muted until ${new Date(state.mute_until * 1000).toLocaleString()}`;
}
