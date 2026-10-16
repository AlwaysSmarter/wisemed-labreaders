<?php
declare(strict_types=1);

/** Server-side helper only. The caller must authenticate and authorize the user first. */
function wsmBrowserToken(array $config, string $subject, array $scopes, int $ttl = 300): string
{
    foreach (['kid', 'tenant_id', 'issuer', 'audience'] as $field) {
        if (empty($config[$field]) || !is_string($config[$field])) {
            throw new InvalidArgumentException('Missing WSM server configuration: ' . $field);
        }
    }
    if (!preg_match('/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/D', $subject)) {
        throw new InvalidArgumentException('Use an internal user identifier for subject.');
    }
    if ($ttl < 1 || $ttl > (int)($config['max_token_ttl_seconds'] ?? 900) || !$scopes) {
        throw new InvalidArgumentException('Invalid token lifetime or scopes.');
    }
    // This allowlist is trusted backend configuration, never request input.
    foreach ($scopes as $scope) {
        if (!is_string($scope) || !in_array($scope, $config['allowed_scopes'] ?? [], true)) {
            throw new InvalidArgumentException('Scope not authorized by backend configuration.');
        }
    }
    $hasSecret = array_key_exists('secret', $config);
    $hasFile = array_key_exists('secret_file', $config);
    if ($hasSecret === $hasFile) {
        throw new InvalidArgumentException('Configure exactly one of secret or secret_file.');
    }
    if ($hasSecret) {
        if (!is_string($config['secret'])) throw new InvalidArgumentException('Secret must be a string.');
        $raw = $config['secret'];
    } else {
        if (!is_string($config['secret_file']) || $config['secret_file'] === '') {
            throw new InvalidArgumentException('Secret file path must be a nonempty string.');
        }
        $raw = file_get_contents($config['secret_file']);
        if ($raw === false) throw new RuntimeException('Cannot load WSM signing key.');
    }
    $secret = trim($raw); // WSM also trims the key file; use the text as-is, not base64-decoded.
    if (strlen($secret) < 32) throw new RuntimeException('WSM key must contain at least 32 bytes.');
    $now = time();
    $header = ['alg' => 'HS256', 'typ' => 'JWT', 'kid' => $config['kid']];
    $claims = [
        'iss' => $config['issuer'], 'aud' => $config['audience'],
        'sub' => $subject, 'tenant_id' => $config['tenant_id'],
        'role' => 'browser', 'client_id' => 'web-' . bin2hex(random_bytes(16)),
        'scopes' => array_values(array_unique($scopes)),
        'iat' => $now, 'nbf' => $now, 'exp' => $now + $ttl,
    ];
    $b64url = static function (string $bytes): string {
        return rtrim(strtr(base64_encode($bytes), '+/', '-_'), '=');
    };
    $input = $b64url(json_encode($header, JSON_THROW_ON_ERROR)) . '.' .
        $b64url(json_encode($claims, JSON_THROW_ON_ERROR));
    return $input . '.' . $b64url(hash_hmac('sha256', $input, $secret, true));
}
