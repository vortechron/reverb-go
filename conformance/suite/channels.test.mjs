import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { collector, sleep } from '../lib/collect.mjs';
import { trigger } from '../lib/signing.mjs';
import { STACKS } from '../lib/stacks.mjs';
import { setup } from './helpers.mjs';

const ctx = setup();
const EVENT = 'App\\Events\\CallEnded';
let n = 0;
const uniq = (base) => `${base}-${process.pid}-${(n += 1)}`;

for (const kind of STACKS) {
  describe(`channels with ${kind}`, () => {
    it('receives an API event on a public channel (name with backslashes)', async () => {
      const name = uniq('pub');
      const c = await ctx.client(kind);
      const got = collector();
      const ch = c.channel('public', name).listen(EVENT, (d) => got.push(d));
      await ch.subscribed();
      const res = await trigger(name, EVENT, { id: 7, msg: 'hi' });
      assert.equal(res.status, 200);
      assert.deepEqual(await got.next('event'), { id: 7, msg: 'hi' });
    });

    it('receives events on a private channel through the auth endpoint (form body)', async () => {
      const name = uniq('priv');
      const c = await ctx.client(kind);
      const got = collector();
      const ch = c.channel('private', name).listen('Ping', (d) => got.push(d));
      await ch.subscribed();
      assert.equal(ctx.auth.seen.at(-1).kind, 'form');
      assert.equal(ctx.auth.seen.at(-1).channel_name, `private-${name}`);
      await trigger(`private-${name}`, 'Ping', { ok: true });
      assert.deepEqual(await got.next('event'), { ok: true });
    });

    it('works with a JSON-body authorizer like terapeas-portal', async () => {
      const name = uniq('json');
      const c = await ctx.client(kind, { json: true });
      const got = collector();
      const ch = c.channel('private', name).listen('Ping', (d) => got.push(d));
      await ch.subscribed();
      assert.equal(ctx.auth.seen.at(-1).kind, 'json');
      await trigger(`private-${name}`, 'Ping', { viaJson: 1 });
      assert.deepEqual(await got.next('event'), { viaJson: 1 });
    });

    it('does not deliver an event to the excluded socket_id', async () => {
      const name = uniq('excl');
      const a = await ctx.client(kind);
      const b = await ctx.client(kind);
      const gotA = collector();
      const gotB = collector();
      const ha = a.channel('public', name).listen('E', (d) => gotA.push(d));
      const hb = b.channel('public', name).listen('E', (d) => gotB.push(d));
      await Promise.all([ha.subscribed(), hb.subscribed()]);
      await trigger(name, 'E', { n: 1 }, { socket_id: a.socketId });
      assert.deepEqual(await gotB.next('event on b'), { n: 1 });
      await gotA.none();
    });

    it('delivers one event to several channels in one API call', async () => {
      const [x, y] = [uniq('multi-x'), uniq('multi-y')];
      const c = await ctx.client(kind);
      const got = collector();
      const hx = c.channel('public', x).listen('M', (d) => got.push(['x', d]));
      const hy = c.channel('public', y).listen('M', (d) => got.push(['y', d]));
      await Promise.all([hx.subscribed(), hy.subscribed()]);
      const res = await (await import('../lib/signing.mjs')).api('POST', '/events', {
        body: { name: 'M', channels: [x, y], data: JSON.stringify({ k: 1 }) },
      });
      assert.equal(res.status, 200);
      await got.next('first');
      await sleep(200);
      assert.deepEqual(got.items.map((i) => i[0]).sort(), ['x', 'y']);
    });

    it('stops delivering after unsubscribe', async () => {
      const name = uniq('unsub');
      const c = await ctx.client(kind);
      const got = collector();
      const ch = c.channel('public', name).listen('E', (d) => got.push(d));
      await ch.subscribed();
      await trigger(name, 'E', { n: 1 });
      await got.next('first event');
      c.leave('public', name);
      await sleep(300);
      await trigger(name, 'E', { n: 2 });
      await got.none();
    });

    it('whispers between two clients on a private channel, not back to the sender', async () => {
      const name = uniq('calls');
      const a = await ctx.client(kind);
      const b = await ctx.client(kind);
      const gotA = collector();
      const gotB = collector();
      const ha = a.channel('private', name);
      const hb = b.channel('private', name);
      ha.listenForWhisper('EndCall', (d) => gotA.push(d));
      hb.listenForWhisper('EndCall', (d) => gotB.push(d));
      await Promise.all([ha.subscribed(), hb.subscribed()]);
      ha.whisper('EndCall', { reason: 'busy' });
      assert.deepEqual(await gotB.next('whisper'), { reason: 'busy' });
      await gotA.none();
    });

    it('tracks presence: here, joining, leaving', async () => {
      const name = uniq('room');
      const a = await ctx.client(kind, { query: '?user=1&name=Alice' });
      const b = await ctx.client(kind, { query: '?user=2&name=Bob' });
      const ha = a.channel('presence', name);
      const joined = collector();
      const left = collector();
      ha.onMemberAdded((m) => joined.push(m));
      ha.onMemberRemoved((m) => left.push(m));
      const first = await ha.subscribed();
      assert.equal(first.count, 1);
      assert.deepEqual(first.members['1'], { name: 'Alice' });

      const hb = b.channel('presence', name);
      const second = await hb.subscribed();
      assert.equal(second.count, 2);
      assert.deepEqual(second.members['2'], { name: 'Bob' });
      const added = await joined.next('member_added');
      assert.equal(String(added.id), '2');
      assert.deepEqual(added.info, { name: 'Bob' });

      b.disconnect();
      const removed = await left.next('member_removed');
      assert.equal(String(removed.id), '2');
    });

    it('counts one user with two sockets as one member', async () => {
      const name = uniq('room2');
      const a = await ctx.client(kind, { query: '?user=1&name=Alice' });
      const b1 = await ctx.client(kind, { query: '?user=2&name=Bob' });
      const b2 = await ctx.client(kind, { query: '?user=2&name=Bob' });
      const ha = a.channel('presence', name);
      const joined = collector();
      const left = collector();
      ha.onMemberAdded((m) => joined.push(m));
      ha.onMemberRemoved((m) => left.push(m));
      await ha.subscribed();
      await b1.channel('presence', name).subscribed();
      await b2.channel('presence', name).subscribed();
      await joined.next('first join');
      await joined.none(500);
      b1.disconnect();
      await left.none(500);
      b2.disconnect();
      await left.next('member_removed after the last socket left');
    });

    it('replays the last event on a cache channel', async () => {
      const name = uniq('cache');
      const full = `cache-${name}`;
      const a = await ctx.client(kind);
      const gotA = collector();
      const ha = a.channel('public', full).listen('Price', (d) => gotA.push(d));
      await ha.subscribed();
      await trigger(full, 'Price', { p: 1 });
      await gotA.next('live event');
      const b = await ctx.client(kind);
      const gotB = collector();
      b.channel('public', full).listen('Price', (d) => gotB.push(d));
      assert.deepEqual(await gotB.next('cached event'), { p: 1 });
    });
  });
}

describe('laravel-echo event name rules', () => {
  for (const kind of ['echo-1.x', 'echo-2.x']) {
    it(`${kind}: listen("CallEnded") matches App\\Events\\CallEnded`, async () => {
      const name = uniq('ns');
      const c = await ctx.client(kind);
      const got = collector();
      const ch = c.echo.channel(name).listen('CallEnded', (d) => got.push(d));
      await new Promise((r) => c.pusher.channel(name).bind('pusher:subscription_succeeded', r));
      await trigger(name, EVENT, { via: 'namespace' });
      assert.deepEqual(await got.next('event'), { via: 'namespace' });
      assert.ok(ch);
    });
  }
});
