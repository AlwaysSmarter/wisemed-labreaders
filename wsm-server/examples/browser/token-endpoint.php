<?php
declare(strict_types=1);
require_once __DIR__ . '/wsm-token.php';

// Integrate with WiseMED's real session/bootstrap and authorization checks.
// These two session fields are illustrative. They MUST be set by trusted login/
// authorization code, never copied from a request parameter.
session_start();
header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');
if (empty($_SESSION['user_id']) || ($_SESSION['wsm_access_allowed'] ?? false) !== true) {
    http_response_code(403);
    echo json_encode(['error' => 'WiseMED login and WSM permission required']);
    exit;
}
try {
    $scopes = ['route:reply', 'route:event', 'route:command', 'connections:read',
               'server:status', 'api:invoke', 'topics:subscribe'];
    // Select tenant/config from the authenticated user's server-side context in
    // multi-tenant WiseMED; this example is for one configured WiseMED instance.
    $config = [
        'kid' => getenv('WSM_KEY_ID'), 'tenant_id' => getenv('WSM_TENANT_ID'),
        'issuer' => getenv('WSM_ISSUER'), 'audience' => 'wsm-server',
        'secret_file' => getenv('WSM_SECRET_FILE'), 'allowed_scopes' => $scopes,
    ];
    echo json_encode(['token' => wsmBrowserToken($config, 'user-' . $_SESSION['user_id'], $scopes)], JSON_THROW_ON_ERROR);
} catch (Throwable $e) {
    http_response_code(500);
    echo json_encode(['error' => 'WSM token issuance failed']);
}
