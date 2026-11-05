// Pusher signing written independently from the Go code, so the suite can
// catch mistakes on either side.
import { createHash, createHmac } from 'node:crypto';
import { httpBase, target } from './env.mjs';

export const hmac = (secret, text) => createHmac('sha256', secret).update(text).digest('hex');

export function channelAuth(socketId, channel, channelData) {
  const text = channelData ? `${socketId}:${channel}:${channelData}` : `${socketId}:${channel}`;
  return `${target.key}:${hmac(target.secret, text)}`;
}

export function signQuery(method, path, params, body) {
  const merged = { ...params };
  if (body) merged.body_md5 = createHash('md5').update(body).digest('hex');
  const canonical = Object.keys(merged).sort().map((k) => `${k}=${merged[k]}`).join('&');
  return hmac(target.secret, `${method}\n${path}\n${canonical}`);
}

/** Calls the HTTP API with a valid signature. `tamper` can break it on purpose. */
export async function api(method, path, { query = {}, body, timestamp, tamper } = {}) {
  const raw = body === undefined ? '' : JSON.stringify(body);
  const params = {
    ...query,
    auth_key: target.key,
    auth_timestamp: String(timestamp ?? Math.floor(Date.now() / 1000)),
    auth_version: '1.0',
  };
  if (raw) params.body_md5 = createHash('md5').update(raw).digest('hex');
  const full = `/apps/${target.appId}${path}`;
  params.auth_signature = signQuery(method, full, Object.fromEntries(Object.entries(params).filter(([k]) => k !== 'body_md5')), raw);
  if (tamper) tamper(params);
  const res = await fetch(`${httpBase}${full}?${new URLSearchParams(params)}`, {
    method,
    headers: raw ? { 'content-type': 'application/json' } : {},
    body: raw || undefined,
  });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* not JSON */ }
  return { status: res.status, json, text };
}

export const trigger = (channel, name, data, extra = {}) =>
  api('POST', '/events', { body: { name, channel, data: JSON.stringify(data), ...extra } });
