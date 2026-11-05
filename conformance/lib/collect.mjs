// Small helpers to wait for events with a timeout.
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export function collector() {
  const items = [];
  const waiters = [];
  let cursor = 0; // items before the cursor were already consumed
  return {
    items,
    push(item) {
      items.push(item);
      waiters.splice(0).forEach((w) => w());
    },
    /** Returns the next item that was not consumed yet, waiting if needed. */
    async next(label, ms = 4000) {
      const deadline = Date.now() + ms;
      while (items.length <= cursor && Date.now() < deadline) {
        await Promise.race([new Promise((r) => waiters.push(r)), sleep(50)]);
      }
      if (items.length <= cursor) throw new Error(`timed out waiting for ${label}`);
      const item = items[cursor];
      cursor += 1;
      return item;
    },
    /** Fails when anything unconsumed arrives within `ms`. */
    async none(ms = 400) {
      await sleep(ms);
      if (items.length > cursor) throw new Error(`unexpected event: ${JSON.stringify(items[cursor])}`);
    },
  };
}

export function once(bindFn, label, ms = 4000) {
  return new Promise((resolve, reject) => {
    const t = setTimeout(() => reject(new Error(`timed out waiting for ${label}`)), ms);
    bindFn((value) => { clearTimeout(t); resolve(value); });
  });
}
