const key = 'wisp.sidebarWidth.v1';
export function installSidebarResize(app, handle) {
  let preferred = 360;
  try { const saved = Number(localStorage.getItem(key)); if (saved >= 260) preferred = saved; } catch { /* optional */ }
  const maximum = () => Math.max(260, Math.min(600, app.clientWidth - 360));
  const apply = () => {
    const value = Math.max(260, Math.min(maximum(), preferred));
    app.style.setProperty('--sidebar-width', `${value}px`);
    handle.setAttribute('aria-valuenow', String(value));
    handle.setAttribute('aria-valuemin', '260');
    handle.setAttribute('aria-valuemax', String(maximum()));
  };
  const set = value => { preferred = Math.round(Math.max(260, Math.min(maximum(), value))); apply(); };
  const save = () => { try { localStorage.setItem(key, String(preferred)); } catch { /* optional */ } };
  handle.addEventListener('pointerdown', e => {
    if (e.button !== 0) return;
    e.preventDefault(); handle.setPointerCapture(e.pointerId); handle.focus();
    app.classList.add('resizing');
  });
  handle.addEventListener('pointermove', e => { if (handle.hasPointerCapture(e.pointerId)) set(e.clientX - app.getBoundingClientRect().left); });
  const finish = () => { app.classList.remove('resizing'); save(); };
  handle.addEventListener('pointerup', finish); handle.addEventListener('lostpointercapture', finish);
  handle.addEventListener('keydown', e => {
    if (!['ArrowLeft','ArrowRight','Home','End'].includes(e.key)) return;
    e.preventDefault(); e.stopPropagation();
    set(e.key === 'Home' ? 260 : e.key === 'End' ? maximum() : preferred + (e.key === 'ArrowLeft' ? -1 : 1) * (e.shiftKey ? 40 : 10)); save();
  });
  window.addEventListener('resize', apply); apply();
}
