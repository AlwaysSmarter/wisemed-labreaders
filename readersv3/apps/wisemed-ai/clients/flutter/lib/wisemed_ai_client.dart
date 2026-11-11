import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;

/// API v1; standalone auth tokens are held in memory. STT runs on the device.
class WiseMedAIClient {
  final Uri base;
  final http.Client _http;
  String? _token;
  WiseMedAIClient(this.base, {http.Client? client})
      : _http = client ?? http.Client();
  Uri _uri(String path) => base.resolve('v1/$path');
  Future<dynamic> request(String method, String path,
      [Object? body, String? idempotencyKey]) async {
    final req = http.Request(method, _uri(path));
    req.headers['Content-Type'] = 'application/json';
    if (_token != null) req.headers['Authorization'] = 'Bearer $_token';
    if (idempotencyKey != null) req.headers['Idempotency-Key'] = idempotencyKey;
    if (body != null) req.body = jsonEncode(body);
    final response = await http.Response.fromStream(
            await _http.send(req).timeout(const Duration(seconds: 60)))
        .timeout(const Duration(seconds: 60));
    final data = jsonDecode(response.body);
    if (response.statusCode >= 400)
      throw WiseMedAIException(
          response.statusCode, data['error']?.toString() ?? 'request_failed');
    return data;
  }

  Future<Map<String, dynamic>> login(String email, String password,
      {String? tenantId}) async {
    final data = await request('POST', 'auth/login', {
      'email': email,
      'password': password,
      if (tenantId != null) 'tenant_id': tenantId
    });
    _token = data['token'] as String;
    return Map<String, dynamic>.from(data['profile']);
  }

  Future<void> logout() async {
    try {
      await request('POST', 'auth/logout', {});
    } finally {
      _token = null;
    }
  }

  Future<Map<String, dynamic>> createSession(
          String templateVersion, String patientRef,
          {required String idempotencyKey}) async =>
      Map<String, dynamic>.from(await request(
          'POST',
          'sessions',
          {'template_version': templateVersion, 'patient_ref': patientRef},
          idempotencyKey));
  Future<Map<String, dynamic>> session(String id) async =>
      Map<String, dynamic>.from(await request('GET', 'sessions/$id'));
  Future<Map<String, dynamic>> sendChunk(
          String sessionId, TranscriptChunk chunk) async =>
      Map<String, dynamic>.from(await request(
          'POST', 'sessions/$sessionId/transcript-chunks', chunk.toJson()));
  Future<Map<String, dynamic>> editField(
          String id, int revision, String field, String value) async =>
      Map<String, dynamic>.from(await request('POST', 'sessions/$id/field-edit',
          {'revision': revision, 'field': field, 'value': value}));
  Future<Map<String, dynamic>> fieldOptions(
          String id, int revision, String field) async =>
      Map<String, dynamic>.from(await request(
          'POST',
          'sessions/$id/field-options',
          {'revision': revision, 'field': field}));
  Future<Map<String, dynamic>> reopen(String id, int revision) async =>
      Map<String, dynamic>.from(
          await request('POST', 'sessions/$id/reopen', {'revision': revision}));
  Future<Map<String, dynamic>> finalize(String id) async =>
      Map<String, dynamic>.from(
          await request('POST', 'sessions/$id/finalize', {}));
  Future<Map<String, dynamic>> result(String id) async =>
      Map<String, dynamic>.from(await request('GET', 'sessions/$id/result'));

  /// Invoke only after an explicit clinical review confirmation in the app.
  Future<Map<String, dynamic>> approve(String id, int revision,
          {required bool confirmed}) async =>
      Map<String, dynamic>.from(await request('POST', 'sessions/$id/approve',
          {'revision': revision, 'confirmed': confirmed}));
  Future<String> redeemPairing(String token) async =>
      (await request('POST', 'pairing/redeem', {'token': token}))['session_id']
          as String;

  /// One bounded SSE connection. Reconnect with the last event ID on disconnect.
  Stream<Map<String, dynamic>> events(String id, {String? lastEventId}) async* {
    final req = http.Request('GET', _uri('sessions/$id/events'));
    req.headers['Accept'] = 'text/event-stream';
    if (_token != null) req.headers['Authorization'] = 'Bearer $_token';
    if (lastEventId != null) req.headers['Last-Event-ID'] = lastEventId;
    final response = await _http.send(req).timeout(const Duration(seconds: 60));
    if (response.statusCode != 200)
      throw WiseMedAIException(response.statusCode, 'events_failed');
    String? eventId;
    await for (final line in response.stream
        .transform(utf8.decoder)
        .transform(const LineSplitter())) {
      if (line.startsWith('id: ')) eventId = line.substring(4);
      if (line.startsWith('data: '))
        yield {
          ...Map<String, dynamic>.from(jsonDecode(line.substring(6))),
          'event_id': eventId
        };
    }
  }

  void close() => _http.close();
}

class TranscriptChunk {
  final String id, text;
  final String? targetField;
  final int sequence, revision;
  final bool isFinal;
  final DateTime timestamp;
  const TranscriptChunk(
      {this.targetField,
      required this.id,
      required this.sequence,
      required this.revision,
      required this.isFinal,
      required this.timestamp,
      required this.text});
  Map<String, dynamic> toJson() => {
        'id': id,
        if (targetField != null) 'target_field': targetField,
        'sequence': sequence,
        'revision': revision,
        'is_final': isFinal,
        'timestamp': timestamp.toUtc().toIso8601String(),
        'text': text
      };
}

class WiseMedAIException implements Exception {
  final int status;
  final String code;
  const WiseMedAIException(this.status, this.code);
  @override
  String toString() => 'WiseMedAIException($status, $code)';
}
