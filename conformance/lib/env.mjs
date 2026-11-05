// Target server settings. The run scripts set these for you.
export const target = {
  name: process.env.CONF_TARGET ?? 'unknown',
  host: process.env.CONF_HOST ?? '127.0.0.1',
  port: Number(process.env.CONF_PORT ?? 6002),
  appId: process.env.CONF_APP_ID ?? 'conf-app',
  key: process.env.CONF_APP_KEY ?? 'conf-key',
  secret: process.env.CONF_APP_SECRET ?? 'conf-secret',
};

export const httpBase = `http://${target.host}:${target.port}`;
