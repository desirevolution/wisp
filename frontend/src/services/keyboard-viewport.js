// Native interactive-widget handles browsers that resize the layout viewport.
// This fallback uses the visible viewport while editing, without hijacking zoom.
export function attachKeyboardViewport(root, { win = window } = {}) {
  const doc = root.ownerDocument, viewport = win.visualViewport;
  let frame = null, disposed = false, last = '';
  function apply() {
    frame = null;
    if (disposed) return;
    const active = doc.activeElement;
    const editing = active && root.contains(active) && active.matches('textarea,input:not([type="checkbox"]):not([type="radio"]):not([type="file"]):not([type="button"]),[contenteditable="true"]');
    const zoomed = viewport && Math.abs((viewport.scale || 1) - 1) > 0.01;
    const usable = viewport && editing && !zoomed && Number.isFinite(viewport.height) && viewport.height > 0;
    const height = usable ? `${viewport.height}px` : '';
    const top = usable ? `${Math.max(0, viewport.offsetTop || 0)}px` : '';
    const signature = `${height}/${top}/${win.innerHeight}`;
    if (signature === last) return;
    last = signature;
    const messages = root.querySelector('#messages');
    const atBottom = messages && messages.scrollHeight - messages.scrollTop - messages.clientHeight < 80;
    const scrollTop = messages?.scrollTop;
    if (height) {
      root.style.setProperty('--app-viewport-height', height);
      root.style.setProperty('--app-viewport-top', top);
    } else {
      root.style.removeProperty('--app-viewport-height');
      root.style.removeProperty('--app-viewport-top');
    }
    // Read after applying layout so following the latest message survives resize.
    // Preserve the reader's position when they have scrolled into history.
    if (messages && !zoomed) messages.scrollTop = atBottom ? Math.max(0, messages.scrollHeight - messages.clientHeight) : scrollTop;
    root.dispatchEvent(new win.Event('viewport-layout'));
  }
  function schedule() { if (frame === null && !disposed) frame = win.requestAnimationFrame(apply); }
  viewport?.addEventListener('resize', schedule);
  viewport?.addEventListener('scroll', schedule);
  win.addEventListener('resize', schedule);
  win.addEventListener('orientationchange', schedule);
  root.addEventListener('focusin', schedule);
  root.addEventListener('focusout', schedule);
  schedule();
  return () => {
    disposed = true;
    if (frame !== null) win.cancelAnimationFrame(frame);
    viewport?.removeEventListener('resize', schedule);
    viewport?.removeEventListener('scroll', schedule);
    win.removeEventListener('resize', schedule);
    win.removeEventListener('orientationchange', schedule);
    root.removeEventListener('focusin', schedule);
    root.removeEventListener('focusout', schedule);
    root.style.removeProperty('--app-viewport-height');
    root.style.removeProperty('--app-viewport-top');
  };
}
