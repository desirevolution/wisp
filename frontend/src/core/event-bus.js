/** Small typed-by-convention application event bus.
 * Keeps transport details (SSE today, something else later) out of UI orchestration.
 */
export function createEventBus() {
  const target = new EventTarget();
  return {
    on(type, listener) {
      const handler = event => listener(event.detail);
      target.addEventListener(type, handler);
      return () => target.removeEventListener(type, handler);
    },
    emit(type, detail) {
      target.dispatchEvent(new CustomEvent(type, { detail }));
    },
  };
}
