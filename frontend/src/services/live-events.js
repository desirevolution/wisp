const SERVER_EVENTS = ['connection', 'message', 'media', 'metadata', 'reaction', 'deletion', 'edit'];

/** Bridges the server's SSE stream to application events.
 * Consumers never need to know about EventSource or JSON framing.
 */
export function connectLiveEvents(bus, url = '/api/events') {
  const source = new EventSource(url);

  source.addEventListener('open', () => bus.emit('live:open'));
  source.addEventListener('error', () => bus.emit('live:error'));

  for (const type of SERVER_EVENTS) {
    source.addEventListener(type, event => {
      try {
        bus.emit(`live:${type}`, JSON.parse(event.data));
      } catch (error) {
        bus.emit('live:parse-error', { type, error });
      }
    });
  }

  return () => source.close();
}
