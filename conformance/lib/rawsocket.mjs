// A bare WebSocket, to look at the exact frames the server sends.
import WebSocket from 'ws';
import { target } from './env.mjs';
import { collector } from './collect.mjs';

export async function rawSocket(key = target.key, headers = {}) {
  const url = `ws://${target.host}:${target.port}/app/${key}?protocol=7&client=js&version=8.4.0&flash=false`;
  const ws = new WebSocket(url, { headers });
  const frames = collector();
  const closed = new Promise((resolve) => ws.on('close', (code) => resolve(code)));
  ws.on('message', (m) => frames.push(JSON.parse(m.toString())));
  await new Promise((resolve, reject) => { ws.once('open', resolve); ws.once('error', reject); });
  const parse = (f) => (typeof f.data === 'string' && f.data ? JSON.parse(f.data) : f.data);
  return {
    ws,
    frames,
    closed,
    parse,
    send: (obj) => ws.send(JSON.stringify(obj)),
    async established() {
      const f = await frames.next('connection_established');
      return { frame: f, data: parse(f) };
    },
    close: () => ws.terminate(),
  };
}
