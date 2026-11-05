import { after, before } from 'node:test';
import { startAuthServer } from '../lib/authserver.mjs';
import { Client } from '../lib/stacks.mjs';

/** Starts the auth server once per test file and closes every client at the end. */
export function setup() {
  const ctx = { clients: [], auth: null };
  before(async () => { ctx.auth = await startAuthServer(); });
  after(() => {
    ctx.clients.forEach((c) => c.disconnect());
    ctx.auth?.close();
  });
  ctx.client = async (kind, options = {}) => {
    const { query = '', connect = true, ...rest } = options;
    const c = new Client(kind, { authUrl: ctx.auth.url + query, ...rest });
    ctx.clients.push(c);
    if (connect) await c.connect();
    return c;
  };
  return ctx;
}
