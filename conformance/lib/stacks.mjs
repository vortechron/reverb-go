// One small client wrapper over three stacks, so every check runs on all of them:
// plain pusher-js 8, laravel-echo 1.x and laravel-echo 2.x (all with pusher-js).
import Echo1Module from 'echo1';
import Echo2 from 'echo2';
import Pusher from 'pusher-js';
import { target } from './env.mjs';
import { collector, once } from './collect.mjs';

// laravel-echo 1.x is CommonJS, so its class can sit under .default.
const Echo1 = Echo1Module.default ?? Echo1Module;

export const STACKS = ['pusher-js', 'echo-1.x', 'echo-2.x'];

const baseOptions = () => ({
  key: target.key,
  wsHost: target.host,
  wsPort: target.port,
  wssPort: target.port,
  forceTLS: false,
  enabledTransports: ['ws'],
  disableStats: true,
  cluster: 'mt1',
});

// The same authorizer shape as terapeas-portal/resources/js/websocket.ts: it POSTs JSON.
const jsonAuthorizer = (authUrl) => (channel) => ({
  authorize: (socketId, callback) => {
    fetch(authUrl, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ socket_id: socketId, channel_name: channel.name }),
    })
      .then((r) => r.json())
      .then((data) => callback(null, data))
      .catch((err) => callback(err));
  },
});

export class Client {
  constructor(kind, { authUrl, json = false, key } = {}) {
    this.kind = kind;
    const options = { ...baseOptions(), ...(key ? { key } : {}) };
    if (authUrl) {
      if (json) options.authorizer = jsonAuthorizer(authUrl);
      else options.authEndpoint = authUrl;
    }
    if (kind === 'pusher-js') {
      this.pusher = new Pusher(options.key, options);
    } else {
      const Echo = kind === 'echo-1.x' ? Echo1 : Echo2;
      this.echo = new Echo({ broadcaster: 'reverb', Pusher, ...options });
      this.pusher = this.echo.connector.pusher;
    }
    this.errors = collector();
    this.pusher.connection.bind('error', (e) => this.errors.push(e));
  }

  get socketId() { return this.pusher.connection.socket_id; }

  connect() {
    if (this.pusher.connection.state === 'connected') return Promise.resolve(this.socketId);
    return once((done) => this.pusher.connection.bind('connected', () => done(this.socketId)), `${this.kind} connected`);
  }

  state() { return this.pusher.connection.state; }

  /** kind: 'public' | 'private' | 'presence'. name has no prefix. */
  channel(type, name) {
    return new Handle(this, type, name);
  }

  leave(type, name) {
    const full = fullName(type, name);
    if (this.echo) this.echo.leave(name);
    else this.pusher.unsubscribe(full);
  }

  disconnect() {
    if (this.echo) this.echo.disconnect();
    else this.pusher.disconnect();
  }
}

const PREFIX = { public: '', private: 'private-', presence: 'presence-' };
export const fullName = (type, name) => PREFIX[type] + name;

class Handle {
  constructor(client, type, name) {
    this.client = client;
    this.type = type;
    this.full = fullName(type, name);
    if (client.echo) {
      const pick = { public: 'channel', private: 'private', presence: 'join' }[type];
      this.ch = client.echo[pick](name);
      this.raw = client.pusher.channel(this.full);
    } else {
      this.ch = client.pusher.subscribe(this.full);
      this.raw = this.ch;
    }
  }

  subscribed() {
    return once((done) => this.raw.bind('pusher:subscription_succeeded', done), `${this.full} subscribed`);
  }

  /** Listen for an exact event name (backslashes allowed). */
  listen(event, cb) {
    if (this.client.echo) this.ch.listen(`.${event}`, cb);
    else this.ch.bind(event, cb);
    return this;
  }

  whisper(event, data) {
    if (this.client.echo) this.ch.whisper(event, data);
    else this.ch.trigger(`client-${event}`, data);
  }

  listenForWhisper(event, cb) {
    if (this.client.echo) this.ch.listenForWhisper(event, cb);
    else this.ch.bind(`client-${event}`, cb);
  }

  members() {
    return new Promise((resolve) => {
      this.raw.bind('pusher:subscription_succeeded', (m) => resolve(m));
    });
  }

  onMemberAdded(cb) { this.raw.bind('pusher:member_added', cb); }
  onMemberRemoved(cb) { this.raw.bind('pusher:member_removed', cb); }
}
