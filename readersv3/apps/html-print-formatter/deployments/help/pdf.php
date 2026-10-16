<?php
// PHP 7.3+ with cURL. Keep this secret only on the PHP server.
$secret = getenv('HTML_PRINT_JWT_SECRET'); // Same literal string as config.yaml.
$kid = getenv('HTML_PRINT_JWT_KEY_ID') ?: 'app-one';
if (!$secret || strlen($secret) < 32) {
    throw new RuntimeException('Set HTML_PRINT_JWT_SECRET (minimum 32 bytes)');
}
$base64url = static function (string $value): string {
    return rtrim(strtr(base64_encode($value), '+/', '-_'), '=');
};
$now = time();
$input = $base64url(json_encode(['alg'=>'HS256', 'typ'=>'JWT', 'kid'=>$kid], JSON_THROW_ON_ERROR)) . '.' .
    $base64url(json_encode(['iss'=>'html-print-formatter', 'aud'=>'html-print-formatter', 'sub'=>'php-client', 'iat'=>$now, 'exp'=>$now+300], JSON_THROW_ON_ERROR));
$jwt = $input . '.' . $base64url(hash_hmac('sha256', $input, $secret, true));
$html = file_get_contents(PHP_SAPI === 'cli' ? ($argv[1] ?? 'report.html') : __DIR__ . '/report.html');
if ($html === false) { throw new RuntimeException('Cannot read HTML'); }
$body = json_encode(['html'=>$html, 'timeout_ms'=>120000], JSON_THROW_ON_ERROR);
// Alternatively: ['html_link'=>'https://example.org/report.html', 'timeout_ms'=>120000]
$curl = curl_init(rtrim(getenv('HTML_PRINT_URL') ?: 'http://127.0.0.1:19113', '/') . '/pdf');
curl_setopt_array($curl, [CURLOPT_POST=>true, CURLOPT_POSTFIELDS=>$body,
    CURLOPT_HTTPHEADER=>['Content-Type: application/json', 'Authorization: Bearer ' . $jwt],
    CURLOPT_RETURNTRANSFER=>true, CURLOPT_CONNECTTIMEOUT=>10, CURLOPT_TIMEOUT=>135]);
$result = curl_exec($curl);
$status = curl_getinfo($curl, CURLINFO_HTTP_CODE);
$error = curl_error($curl);
curl_close($curl);
if ($result === false || $status !== 200) {
    throw new RuntimeException("PDF request failed ($status): " . ($error ?: $result));
}
// Send the PDF directly; no PDF file is written to disk.
header('Content-Type: application/pdf');
header('Content-Disposition: inline; filename="raport.pdf"');
header('Content-Length: ' . strlen($result));
header('Cache-Control: no-store');
echo $result;
exit;
