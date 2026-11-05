<?php

// Drives the real pusher-php-server library, the one Laravel uses to publish to Reverb.
// Usage: php pusher.php '<json command>'
// The library comes from a Laravel app's vendor folder (read only): CONF_PHP_AUTOLOAD.

declare(strict_types=1);

$autoload = getenv('CONF_PHP_AUTOLOAD') ?: '/Users/amiruladli/Projects/werewolf/vendor/autoload.php';
require $autoload;

$pusher = new Pusher\Pusher(
    getenv('CONF_APP_KEY'),
    getenv('CONF_APP_SECRET'),
    getenv('CONF_APP_ID'),
    [
        'host' => getenv('CONF_HOST'),
        'port' => (int) getenv('CONF_PORT'),
        'scheme' => 'http',
        'useTLS' => false,
        'timeout' => 10,
    ],
);

$cmd = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);

// Laravel's PusherBroadcaster passes the payload as an array and the socket id as a param.
$result = match ($cmd['type']) {
    'trigger' => $pusher->trigger(
        $cmd['channels'],
        $cmd['event'],
        $cmd['data'],
        isset($cmd['socket_id']) ? ['socket_id' => $cmd['socket_id']] : [],
    ),
    'batch' => $pusher->triggerBatch($cmd['batch']),
    'channels' => $pusher->getChannels(['info' => 'subscription_count']),
    'private_auth' => json_decode($pusher->authorizeChannel($cmd['channel'], $cmd['socket_id']), true),
    'presence_auth' => json_decode(
        $pusher->authorizePresenceChannel($cmd['channel'], $cmd['socket_id'], (string) $cmd['user_id'], $cmd['user_info']),
        true,
    ),
};

echo json_encode($result, JSON_THROW_ON_ERROR);
