// The real pusher-php-server (what Laravel publishes with) talking to the server under test.
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync } from 'node:fs';
import { describe, it } from 'node:test';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { collector } from '../lib/collect.mjs';
import { target } from '../lib/env.mjs';
import { rawSocket } from '../lib/rawsocket.mjs';
import { setup } from './helpers.mjs';

const run = promisify(execFile);
const script = fileURLToPath(new URL('../php/pusher.php', import.meta.url));
const autoload = process.env.CONF_PHP_AUTOLOAD ?? '/Users/amiruladli/Projects/werewolf/vendor/autoload.php';

const hasPhp = await run('php', ['-v']).then(() => true, () => false);
const skip = !hasPhp
  ? 'php is not installed'
  : !existsSync(autoload) ? `pusher-php-server not found at ${autoload} (set CONF_PHP_AUTOLOAD)` : false;

const ctx = setup();
let n = 0;
const uniq = (base) => `${base}-${process.pid}-${(n += 1)}`;

async function php(cmd) {
  const { stdout } = await run('php', [script, JSON.stringify(cmd)], {
    env: {
      ...process.env,
      CONF_HOST: target.host,
      CONF_PORT: String(target.port),
      CONF_APP_ID: target.appId,
      CONF_APP_KEY: target.key,
      CONF_APP_SECRET: target.secret,
      CONF_PHP_AUTOLOAD: autoload,
    },
  });
  return JSON.parse(stdout);
}

describe('pusher-php-server publishing (the Laravel path)', { skip }, () => {
  it('trigger() delivers a Laravel-style event to a client', async () => {
    const name = uniq('php');
    const c = await ctx.client('echo-2.x');
    const got = collector();
    const ch = c.channel('public', name).listen('App\\Events\\OrderShipped', (d) => got.push(d));
    await ch.subscribed();
    const res = await php({ type: 'trigger', channels: [name], event: 'App\\Events\\OrderShipped', data: { order: { id: 5 }, note: 'é<>&' } });
    assert.ok(res, 'trigger returned');
    assert.deepEqual(await got.next('event'), { order: { id: 5 }, note: 'é<>&' });
  });

  it('trigger() with socket_id skips that socket (Echo toOthers)', async () => {
    const name = uniq('phpex');
    const a = await ctx.client('pusher-js');
    const b = await ctx.client('pusher-js');
    const gotA = collector();
    const gotB = collector();
    const ha = a.channel('public', name).listen('E', (d) => gotA.push(d));
    const hb = b.channel('public', name).listen('E', (d) => gotB.push(d));
    await Promise.all([ha.subscribed(), hb.subscribed()]);
    await php({ type: 'trigger', channels: [name], event: 'E', data: { n: 1 }, socket_id: a.socketId });
    assert.deepEqual(await gotB.next('event'), { n: 1 });
    await gotA.none();
  });

  it('trigger() to many channels and triggerBatch() both arrive', async () => {
    const [x, y] = [uniq('phpx'), uniq('phpy')];
    const c = await ctx.client('pusher-js');
    const got = collector();
    const hx = c.channel('public', x).listen('M', (d) => got.push(['x', d]));
    const hy = c.channel('public', y).listen('B', (d) => got.push(['y', d]));
    await Promise.all([hx.subscribed(), hy.subscribed()]);
    await php({ type: 'trigger', channels: [x, y], event: 'M', data: { k: 1 } });
    await php({ type: 'batch', batch: [{ channel: y, name: 'B', data: { k: 2 } }] });
    await got.next('multi-channel event');
    await got.next('batch event');
    assert.deepEqual(got.items.map((i) => i[0]).sort(), ['x', 'y']);
  });

  it('getChannels() works against the HTTP API', async () => {
    const name = uniq('phpinfo');
    const c = await ctx.client('pusher-js');
    await c.channel('public', name).subscribed();
    const res = await php({ type: 'channels' });
    assert.equal(res.channels[name].subscription_count, 1);
  });

  it('accepts private and presence auth strings made by the PHP library', async () => {
    const s = await rawSocket();
    const { data } = await s.established();
    const priv = await php({ type: 'private_auth', channel: 'private-php', socket_id: data.socket_id });
    s.send({ event: 'pusher:subscribe', data: { channel: 'private-php', auth: priv.auth } });
    assert.equal((await s.frames.next('private subscribed')).event, 'pusher_internal:subscription_succeeded');

    const pres = await php({
      type: 'presence_auth', channel: 'presence-php', socket_id: data.socket_id, user_id: 42, user_info: { name: 'PHP' },
    });
    s.send({ event: 'pusher:subscribe', data: { channel: 'presence-php', auth: pres.auth, channel_data: pres.channel_data } });
    const ok = await s.frames.next('presence subscribed');
    assert.equal(ok.event, 'pusher_internal:subscription_succeeded');
    assert.equal(JSON.parse(ok.data).presence.count, 1);
    s.close();
  });
});
