// Tiny static server for the demo page. Config comes from the environment.
// PORT          port for this page (default 6020)
// WS_HOST/WS_PORT  where reverb-go listens (default 127.0.0.1:6002)
// APP_KEY       the app key (REVERB_APP_KEY)
import { createReadStream } from 'node:fs';
import { createServer } from 'node:http';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const port = Number(process.env.PORT ?? 6020);
const config = {
  key: process.env.APP_KEY ?? 'conf-key',
  wsHost: process.env.WS_HOST ?? '127.0.0.1',
  wsPort: Number(process.env.WS_PORT ?? 6002),
  channel: process.env.CHANNEL ?? 'demo',
};

const files = {
  '/': ['index.html', 'text/html; charset=utf-8'],
  '/vendor/pusher.js': ['node_modules/pusher-js/dist/web/pusher.min.js', 'text/javascript'],
  '/vendor/echo.js': ['node_modules/laravel-echo/dist/echo.iife.js', 'text/javascript'],
};

createServer((req, res) => {
  const path = new URL(req.url, 'http://x').pathname;
  if (path === '/config.json') {
    res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(config));
    return;
  }
  const entry = files[path];
  if (!entry) {
    res.writeHead(404).end('not found');
    return;
  }
  res.writeHead(200, { 'content-type': entry[1] });
  createReadStream(join(here, entry[0])).pipe(res);
}).listen(port, '127.0.0.1', () => console.log(`echo page on http://127.0.0.1:${port}/`));
