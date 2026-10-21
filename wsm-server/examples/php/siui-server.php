<?php
declare(strict_types=1);

// PHP 8.1+. Install alongside this file: composer require phrity/websocket:~3.6.0
require __DIR__ . '/vendor/autoload.php';

use WebSocket\Client;
use WebSocket\Middleware\CloseHandler;
use WebSocket\Middleware\PingResponder;

/** Controls an existing SIUI Bridge over WSM; does not start a bridge process. */
final class SIUIBridge
{
    private Client $socket;
    private string $connectionId;

    public function __construct(string $url, string $jwt, string $clientId, string $equipmentId)
    {
        $this->socket = new Client($url);
        $this->socket->addMiddleware(new CloseHandler());
        $this->socket->addMiddleware(new PingResponder());
        $this->socket->addHeader('Authorization', 'Bearer ' . $jwt);
        $this->socket->setTimeout(10);
        try {
            $this->send([
                'type' => 'hello',
                'payload' => ['client_type' => 'service', 'client_id' => $clientId],
            ]);
            $this->awaitMessage('hello_ack', null, null, 10);

            $list = $this->request('list_connections', [], null, 'connections');
            $matches = array_values(array_filter(
                $list['connections'] ?? [],
                static fn(array $c): bool => ($c['client_type'] ?? '') === 'siui-bridge'
                    && (string) ($c['equipment_id'] ?? '') === $equipmentId
            ));
            if (count($matches) !== 1) {
                throw new RuntimeException('SIUI Bridge offline sau selecție ambiguă.');
            }
            $this->connectionId = $matches[0]['id'];
        } catch (Throwable $error) {
            $this->close();
            throw $error;
        }
    }

    public function call(string $method, string $path, ?array $body = null, int $timeout = 30): array
    {
        $args = ['method' => $method, 'path' => $path];
        if ($body !== null) {
            $args['body'] = $body;
        }
        $response = $this->request('command', [
            'command' => 'api.request', 'args' => $args,
        ], ['mode' => 'connection', 'connection_id' => $this->connectionId], 'reply', $timeout);

        if (($response['kind'] ?? '') !== 'api.response') {
            throw new RuntimeException('Răspuns neașteptat de la SIUI Bridge.');
        }
        // Keep HTTP status and body, including application errors, available to the caller.
        return $response;
    }

    public function getStatus(): array
    {
        return $this->call('GET', '/api/siui/status');
    }

    public function close(): void
    {
        $this->socket->disconnect();
    }

    private function send(array $message): void
    {
        $this->socket->text(json_encode($message, JSON_THROW_ON_ERROR));
    }

    private function request(string $type, array $payload, ?array $target, string $expected, int $timeout = 30): array
    {
        $id = bin2hex(random_bytes(16));
        $message = ['type' => $type, 'request_id' => $id, 'payload' => (object) $payload];
        if ($target !== null) {
            $message['target'] = $target;
        }
        $this->send($message);
        return $this->awaitMessage($expected, $id, $target['connection_id'] ?? null, $timeout);
    }

    private function awaitMessage(string $expected, ?string $id, ?string $peer, int $timeout): array
    {
        $deadline = microtime(true) + $timeout;
        while (($remaining = $deadline - microtime(true)) > 0) {
            $this->socket->setTimeout($remaining);
            $frame = $this->socket->receive();
            if ($frame === null || $frame->getOpcode() === 'close') {
                throw new RuntimeException('Conexiunea WSM a fost închisă. Execuția poate fi deja începută.');
            }
            if ($frame->getOpcode() !== 'text') {
                continue;
            }
            $message = json_decode($frame->getContent(), true, 512, JSON_THROW_ON_ERROR);
            $type = $message['type'] ?? '';
            $payload = $message['payload'] ?? [];
            $sender = $message['connection_id'] ?? $payload['sender_connection_id'] ?? null;
            if ($id !== null && ($message['correlation_id'] ?? $message['request_id'] ?? '') !== $id) {
                continue;
            }
            if ($type === 'error' && !$sender) {
                throw new RuntimeException($payload['message'] ?? 'Eroare WSM.');
            }
            if ($type === 'command_ack' && !$sender && ($payload['recipients'] ?? 0) === 0) {
                throw new RuntimeException('Bridge indisponibil: comanda nu a fost pusă în coadă.');
            }
            // A queue ACK is not the result. Also verify the responding connection.
            if ($type === $expected && ($peer !== null ? $sender === $peer : !$sender)) {
                if (($payload['ok'] ?? true) === false) {
                    throw new RuntimeException('Comandă refuzată: ' . json_encode($payload['error'] ?? null));
                }
                return $payload;
            }
        }
        throw new RuntimeException('Timeout WSM; nu retrimite automat o comandă cu efecte.');
    }
}

function requiredEnv(string $name): string
{
    $value = getenv($name);
    if ($value === false || $value === '') {
        throw new RuntimeException('Lipsește variabila ' . $name);
    }
    return $value;
}

// CLI demo. When including this file in another PHP script, only the class is loaded.
if (PHP_SAPI === 'cli' && realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $siui = null;
    try {
        $siui = new SIUIBridge(
            requiredEnv('WSM_URL'),
            requiredEnv('WSM_JWT'),
            requiredEnv('WSM_CLIENT_ID'), // Must equal client_id in the JWT.
            requiredEnv('SIUI_EQUIPMENT_ID')
        );
        $result = $siui->getStatus();
        echo json_encode($result, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR), PHP_EOL;
        exit($result['status'] >= 200 && $result['status'] < 300 ? 0 : 1);
    } finally {
        $siui?->close();
    }
}
