import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { httpBase } from '../lib/env.mjs';
import { collector, sleep } from '../lib/collect.mjs';
import { rawSocket } from '../lib/rawsocket.mjs';
import { api, channelAuth, trigger } from '../lib/signing.mjs';
import { setup } from './helpers.mjs';

const ctx = setup();
let n = 0;
const uniq = (base) => `${base}-${process.pid}-${(n += 1)}`;
const body = { name: 'E', channel: 'c', data: '{}' };

describe('HTTP API', () => {
  it('GET /up answers 200', async () => {
    const res = await fetch(`${httpBase}/up`);
    assert.equal(res.status, 200);
  });

  it('accepts a correctly signed event and answers {}', async () => {
    const res = await api('POST', '/events', { body });
    assert.equal(res.status, 200);
    assert.deepEqual(res.json, {});
  });

  it('rejects a wrong signature with 401', async () => {
    const res = await api('POST', '/events', { body, tamper: (p) => { p.auth_signature = '0'.repeat(64); } });
    assert.equal(res.status, 401);
  });

  it('rejects a changed body or query with 401', async () => {
    const res = await api('POST', '/events', { body, tamper: (p) => { p.auth_version = '2.0'; } });
    assert.equal(res.status, 401);
  });

  it('answers 404 for an unknown app id', async () => {
    const res = await fetch(`${httpBase}/apps/nope/events?auth_signature=x`, { method: 'POST', body: '{}' });
    assert.equal(res.status, 404);
  });

  it('answers 422 for an event without data', async () => {
    const res = await api('POST', '/events', { body: { name: 'E', channel: 'c' } });
    assert.equal(res.status, 422);
  });

  it('publishes a batch', async () => {
    const [a, b] = [uniq('batch-a'), uniq('batch-b')];
    const c = await ctx.client('pusher-js');
    const got = collector();
    const ha = c.channel('public', a).listen('B1', (d) => got.push(['a', d]));
    const hb = c.channel('public', b).listen('B2', (d) => got.push(['b', d]));
    await Promise.all([ha.subscribed(), hb.subscribed()]);
    const res = await api('POST', '/batch_events', {
      body: { batch: [
        { name: 'B1', channel: a, data: JSON.stringify({ i: 1 }) },
        { name: 'B2', channel: b, data: JSON.stringify({ i: 2 }) },
      ] },
    });
    assert.equal(res.status, 200);
    await got.next('first');
    await got.next('second');
    assert.deepEqual(got.items.map((i) => i[0]).sort(), ['a', 'b']);
  });

  it('lists channels, one channel and the connection count', async () => {
    const name = uniq('info');
    const c = await ctx.client('pusher-js');
    await c.channel('public', name).subscribed();

    const list = await api('GET', '/channels', { query: { filter_by_prefix: name.slice(0, 8), info: 'subscription_count' } });
    assert.equal(list.status, 200);
    assert.equal(list.json.channels[name].subscription_count, 1);

    const one = await api('GET', `/channels/${name}`, { query: { info: 'subscription_count' } });
    assert.equal(one.status, 200);
    assert.equal(one.json.occupied, true);
    assert.equal(one.json.subscription_count, 1);

    const none = await api('GET', `/channels/${uniq('missing')}`);
    assert.equal(none.status, 200);
    assert.equal(none.json.occupied, false);

    const conns = await api('GET', '/connections');
    assert.equal(conns.status, 200);
    assert.ok(conns.json.connections >= 1);
  });

  it('lists presence users and refuses the list for other channels', async () => {
    const name = uniq('users');
    const c = await ctx.client('pusher-js', { query: '?user=5&name=Eve' });
    await c.channel('presence', name).subscribed();
    const users = await api('GET', `/channels/presence-${name}/users`);
    assert.equal(users.status, 200);
    assert.deepEqual(users.json.users.map((u) => String(u.id)), ['5']);
    const info = await api('GET', `/channels/presence-${name}`, { query: { info: 'user_count' } });
    assert.equal(info.json.user_count, 1);

    const pub = uniq('pubusers');
    await c.channel('public', pub).subscribed();
    assert.equal((await api('GET', `/channels/${pub}/users`)).status, 400);
    assert.equal((await api('GET', `/channels/${uniq('nochan')}/users`)).status, 404);
  });

  it('terminates connections of a user', async () => {
    const name = `presence-${uniq('kick')}`;
    const s = await rawSocket();
    const { data } = await s.established();
    const channelData = JSON.stringify({ user_id: 77, user_info: { name: 'Kick' } });
    s.send({ event: 'pusher:subscribe', data: { channel: name, auth: channelAuth(data.socket_id, name, channelData), channel_data: channelData } });
    assert.equal((await s.frames.next('subscribed')).event, 'pusher_internal:subscription_succeeded');
    const res = await api('POST', '/users/77/terminate_connections');
    assert.equal(res.status, 200);
    const outcome = await Promise.race([s.closed, sleep(4000).then(() => 'still open')]);
    assert.notEqual(outcome, 'still open');
  });

  it('returns channel info with an event when info is requested', async () => {
    const name = uniq('evinfo');
    const c = await ctx.client('pusher-js');
    await c.channel('public', name).subscribed();
    const res = await api('POST', '/events', {
      body: { name: 'E', channels: [name], data: '{}', info: 'subscription_count' },
    });
    assert.equal(res.status, 200);
    assert.equal(res.json.channels[name].subscription_count, 1);
    assert.ok(trigger);
  });
});
