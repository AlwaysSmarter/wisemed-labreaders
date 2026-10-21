<?php
declare(strict_types=1);

final class WSMHttpException extends RuntimeException
{
    public function __construct(
        string $message,
        public readonly string $errorCode,
        public readonly ?string $requestId = null,
        public readonly bool $executionUnknown = false,
        public readonly ?int $httpStatus = null
    ) {
        parent::__construct($message);
    }
}

/** PHP 8.1+ with ext-curl; all timeout values are milliseconds. */
final class WSMHttpClient
{
    private string $baseURL;
    private array $options = [
        'connectTimeout' => 5000,
        'requestTimeout' => 135000,
        'totalTimeout' => 300000,
        'connectAttempts' => 3,
    ];

    public function __construct(string $baseURL, private string $jwt, array $options = [], private ?string $caFile = null)
    {
        if (parse_url($baseURL, PHP_URL_SCHEME) !== 'https') {
            throw new InvalidArgumentException('WSM URL must use HTTPS.');
        }
        $this->baseURL = rtrim($baseURL, '/');
        $this->options = $this->settings($options);
    }

    public function setTimeouts(array $options): self
    {
        if (array_diff(array_keys($options), ['connectTimeout', 'requestTimeout', 'totalTimeout'])) {
            throw new InvalidArgumentException('Unknown timeout option.');
        }
        $this->options = $this->settings($options);
        return $this;
    }

    public function setReconnectOptions(array $options): self
    {
        if (array_diff(array_keys($options), ['connectAttempts', 'connectTimeout'])) {
            throw new InvalidArgumentException('Unknown reconnect option.');
        }
        $this->options = $this->settings($options);
        return $this;
    }

    public function getOptions(): array { return $this->options; }

    /** Lists this tenant's connected clients; null means every client type. */
    public function listClients(?string $clientType = null, array $options = []): array
    {
        $query = $clientType === null ? '' : '?' . http_build_query(['client_type' => $clientType]);
        return $this->request('GET', '/api/connections' . $query, null, $options)['connections'];
    }

    /** First client in WSM order (oldest connection first), without a fixed equipment ID. */
    public function firstClient(string $clientType, array $options = []): array
    {
        $clients = $this->listClients($clientType, $options);
        if (!$clients) {
            throw new WSMHttpException('No connected client of type ' . $clientType, 'client_offline');
        }
        return $clients[0];
    }

    /** Blocks for the actual peer reply. A POST is never retried automatically. */
    public function sendCommand(array $client, string $command, array $args = [], array $options = []): array
    {
        $settings = $this->settings($options);
        if (empty($client['id']) || $command === '') {
            throw new InvalidArgumentException('Client ID and command are required.');
        }
        $result = $this->request('POST', '/api/connections/' . rawurlencode($client['id']) . '/commands', [
            'command' => $command,
            'args' => (object) $args,
            'timeout_ms' => min($settings['requestTimeout'], $settings['totalTimeout']),
        ], $settings);
        $response = $result['response'];
        if (($response['ok'] ?? true) === false) {
            throw new WSMHttpException('Command refused: ' . json_encode($response['error'] ?? null), 'command_failed', $result['request_id']);
        }
        return $response;
    }

    /** Common API adapter; status/body/body_base64 preserve remote HTTP errors. */
    public function callAPI(array $client, string $method, string $path, ?array $body = null, array $options = []): array
    {
        $args = ['method' => strtoupper($method), 'path' => $path];
        if ($body !== null) {
            $args['body'] = $body;
        }
        $response = $this->sendCommand($client, 'api.request', $args, $options);
        if (($response['kind'] ?? '') !== 'api.response') {
            throw new WSMHttpException('Expected api.response.', 'invalid_response');
        }
        return $response;
    }

    private function settings(array $overrides): array
    {
        if (array_diff(array_keys($overrides), array_keys($this->options))) {
            throw new InvalidArgumentException('Unknown WSM option.');
        }
        $settings = array_replace($this->options, $overrides);
        foreach ($settings as $name => $value) {
            if (!is_int($value) || $value <= 0) {
                throw new InvalidArgumentException($name . ' must be a positive integer.');
            }
        }
        if ($settings['requestTimeout'] > 300000) {
            throw new InvalidArgumentException('requestTimeout must be at most 300000 ms.');
        }
        return $settings;
    }

    private function request(string $method, string $path, ?array $body, array $options): array
    {
        $settings = $this->settings($options);
        $deadline = hrtime(true) / 1e6 + $settings['totalTimeout'];
        // GET discovery is safe to retry. Never replay a submitted command.
        $attempts = $method === 'GET' ? $settings['connectAttempts'] : 1;
        $encoded = $body === null ? null : json_encode($body, JSON_THROW_ON_ERROR);
        for ($attempt = 1; $attempt <= $attempts; $attempt++) {
            $remaining = (int) floor($deadline - hrtime(true) / 1e6);
            if ($remaining <= 0) {
                throw new WSMHttpException('Total waiting time expired.', 'operation_timeout');
            }
            $handle = curl_init($this->baseURL . $path);
            if ($handle === false) {
                throw new WSMHttpException('Cannot initialize cURL.', 'curl_init_failed');
            }
            try {
                curl_setopt_array($handle, [
                    CURLOPT_CUSTOMREQUEST => $method,
                    CURLOPT_RETURNTRANSFER => true,
                    CURLOPT_FOLLOWLOCATION => false,
                    CURLOPT_CONNECTTIMEOUT_MS => min($settings['connectTimeout'], $remaining),
                    // Allow WSM time to serialize its timeout response; totalTimeout remains binding.
                    CURLOPT_TIMEOUT_MS => min($settings['requestTimeout'] + 5000, $remaining),
                    CURLOPT_SSL_VERIFYPEER => true,
                    CURLOPT_SSL_VERIFYHOST => 2,
                    CURLOPT_HTTPHEADER => [
                        'Authorization: Bearer ' . $this->jwt,
                        'Accept: application/json',
                        'Content-Type: application/json',
                    ],
                ]);
                if ($this->caFile !== null) {
                    curl_setopt($handle, CURLOPT_CAINFO, $this->caFile);
                }
                if ($encoded !== null) {
                    curl_setopt($handle, CURLOPT_POSTFIELDS, $encoded);
                }
                $raw = curl_exec($handle);
                if ($raw === false) {
                    $errno = curl_errno($handle);
                    // Retry only transient errors on read-only discovery requests.
                    if ($attempt < $attempts && in_array($errno, [6, 7, 28, 52, 55, 56], true)) {
                        continue;
                    }
                    throw new WSMHttpException(
                        'WSM transport failed (cURL ' . $errno . '); command execution may be unknown.',
                        $errno === 28 ? 'request_timeout' : 'wsm_disconnected',
                        executionUnknown: $method === 'POST'
                    );
                }
                $status = curl_getinfo($handle, CURLINFO_RESPONSE_CODE);
                if ($status < 200 || $status >= 300) {
                    $error = json_decode($raw, true);
                    $code = is_array($error) ? ($error['error'] ?? 'request_failed') : 'request_failed';
                    throw new WSMHttpException(
                        'WSM HTTP ' . $status . ': ' . $code,
                        $code,
                        is_array($error) ? ($error['request_id'] ?? null) : null,
                        str_contains($code, 'execution_unknown'),
                        $status
                    );
                }
                return json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
            } finally {
                curl_close($handle);
            }
        }
        throw new WSMHttpException('Connection attempts exhausted.', 'wsm_disconnected');
    }
}
