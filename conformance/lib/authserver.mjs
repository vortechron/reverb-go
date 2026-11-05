// A tiny /broadcasting/auth endpoint, like the one a Laravel app provides.
// Query: ?user=<id>&name=<name> picks the presence user, ?bad=1 forges a bad signature.
import { createServer } from 'node:http';
import { channelAuth } from './signing.mjs';

async function readBody(req) {
  let text = '';
  for await (const chunk of req) text += chunk;
  const type = req.headers['content-type'] ?? '';
  if (type.startsWith('application/json')) return { kind: 'json', fields: JSON.parse(text) };
  return { kind: 'form', fields: Object.fromEntries(new URLSearchParams(text)) };
}

export async function startAuthServer(firstPort = 6010) {
  const seen = [];
  const server = createServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://x');
      if (req.method !== 'POST' || !url.pathname.endsWith('/broadcasting/auth')) {
        res.writeHead(404).end();
        return;
      }
      const { kind, fields } = await readBody(req);
      seen.push({ kind, ...fields });
      const { socket_id: socketId, channel_name: channel } = fields;
      let body;
      if (channel.startsWith('presence-')) {
        const data = JSON.stringify({
          user_id: Number(url.searchParams.get('user') ?? 1),
          user_info: { name: url.searchParams.get('name') ?? 'user' },
        });
        body = { auth: channelAuth(socketId, channel, data), channel_data: data };
      } else {
        body = { auth: channelAuth(socketId, channel) };
      }
      if (url.searchParams.get('bad')) body.auth = body.auth.replace(/.$/, (c) => (c === '0' ? '1' : '0'));
      res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(body));
    } catch (err) {
      res.writeHead(500).end(String(err));
    }
  });
  for (let port = firstPort; port < 6100; port += 1) {
    const ok = await new Promise((resolve) => {
      server.once('error', () => resolve(false));
      server.listen(port, '127.0.0.1', () => resolve(true));
    });
    if (ok) return { url: `http://127.0.0.1:${port}/broadcasting/auth`, seen, close: () => server.close() };
  }
  throw new Error('no free port in 6010-6099 for the auth server');
}
