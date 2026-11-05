import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { rawSocket } from '../lib/rawsocket.mjs';
import { channelAuth } from '../lib/signing.mjs';
import { STACKS } from '../lib/stacks.mjs';
import { setup } from './helpers.mjs';

const ctx = setup();

// pusher-js wraps the server's pusher:error differently across versions.
const errorCode = (e) => e?.error?.data?.code ?? e?.data?.code ?? e?.code;

describe('connection (raw frames)', () => {
  it('sends connection_established with a socket id and activity timeout', async () => {
    const s = await rawSocket();
    const { frame, data } = await s.established();
    assert.equal(frame.event, 'pusher:connection_established');
    assert.equal(typeof frame.data, 'string', 'data is a JSON string');
    assert.match(data.socket_id, /^\d+\.\d+$/);
    assert.ok(data.activity_timeout > 0);
    s.close();
  });

  it('answers pusher:ping with pusher:pong', async () => {
    const s = await rawSocket();
    await s.established();
    s.send({ event: 'pusher:ping' });
    const f = await s.frames.next('pong');
    assert.equal(f.event, 'pusher:pong');
    s.close();
  });

  it('answers a public subscribe with an empty subscription_succeeded', async () => {
    const s = await rawSocket();
    await s.established();
    s.send({ event: 'pusher:subscribe', data: { channel: 'news' } });
    const f = await s.frames.next('subscription_succeeded');
    assert.equal(f.event, 'pusher_internal:subscription_succeeded');
    assert.equal(f.channel, 'news');
    assert.equal(f.data, '{}');
    s.close();
  });

  it('sends pusher:error 4001 for an unknown app key', async () => {
    const s = await rawSocket('no-such-key');
    const f = await s.frames.next('error');
    assert.equal(f.event, 'pusher:error');
    assert.equal(s.parse(f).code, 4001);
    s.close();
  });

  it('sends pusher:error 4009 for a bad private channel signature', async () => {
    const s = await rawSocket();
    const { data } = await s.established();
    s.send({ event: 'pusher:subscribe', data: { channel: 'private-x', auth: `${'k'}:deadbeef` } });
    const f = await s.frames.next('error');
    assert.equal(f.event, 'pusher:error');
    assert.equal(s.parse(f).code, 4009);
    s.send({ event: 'pusher:subscribe', data: { channel: 'private-x', auth: channelAuth(data.socket_id, 'private-x') } });
    const ok = await s.frames.next('subscription_succeeded after a good signature');
    assert.equal(ok.event, 'pusher_internal:subscription_succeeded');
    s.close();
  });

  it('rejects a private signature made for another channel or socket', async () => {
    const s = await rawSocket();
    const { data } = await s.established();
    s.send({ event: 'pusher:subscribe', data: { channel: 'private-a', auth: channelAuth(data.socket_id, 'private-b') } });
    assert.equal(s.parse(await s.frames.next('error')).code, 4009);
    s.send({ event: 'pusher:subscribe', data: { channel: 'private-a', auth: channelAuth('1.2', 'private-a') } });
    assert.equal(s.parse(await s.frames.next('error')).code, 4009);
    s.close();
  });

  it('rejects tampered presence channel_data', async () => {
    const s = await rawSocket();
    const { data } = await s.established();
    const signed = JSON.stringify({ user_id: 1 });
    s.send({ event: 'pusher:subscribe', data: { channel: 'presence-r', auth: channelAuth(data.socket_id, 'presence-r', signed), channel_data: JSON.stringify({ user_id: 99 }) } });
    assert.equal(s.parse(await s.frames.next('error')).code, 4009);
    s.close();
  });

  it('answers garbage with pusher:error 4200', async () => {
    const s = await rawSocket();
    await s.established();
    s.ws.send('this is not json');
    assert.equal(s.parse(await s.frames.next('error')).code, 4200);
    s.close();
  });

  it('refuses a client event from a socket that is not subscribed', async () => {
    const s = await rawSocket();
    await s.established();
    s.send({ event: 'client-hello', channel: 'private-x', data: { a: 1 } });
    assert.equal(s.parse(await s.frames.next('error')).code, 4009);
    s.close();
  });
});

for (const kind of STACKS) {
  describe(`connection with ${kind}`, () => {
    it('connects and gets a socket id', async () => {
      const c = await ctx.client(kind);
      assert.equal(c.state(), 'connected');
      assert.match(c.socketId, /^\d+\.\d+$/);
    });

    it('reports a wrong app key as error 4001', async () => {
      const c = await ctx.client(kind, { key: 'wrong-key', connect: false });
      const e = await c.errors.next('connection error');
      assert.equal(errorCode(e), 4001);
      assert.notEqual(c.state(), 'connected');
    });

    it('reports a bad auth signature as error 4009 and keeps the connection', async () => {
      const c = await ctx.client(kind, { query: '?bad=1' });
      const h = c.channel('private', 'forged');
      const e = await c.errors.next('auth error');
      assert.equal(errorCode(e), 4009);
      assert.equal(c.state(), 'connected');
      assert.ok(h);
    });
  });
}
