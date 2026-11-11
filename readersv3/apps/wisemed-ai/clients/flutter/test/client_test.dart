import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';
import 'package:wisemed_ai_client/wisemed_ai_client.dart';

void main() {
  test('login token remains private and is attached to later API calls',
      () async {
    final requests = <http.Request>[];
    final client = WiseMedAIClient(Uri.parse('http://127.0.0.1:19120/'),
        client: MockClient((req) async {
      requests.add(req);
      if (req.url.path.endsWith('/auth/login'))
        return http.Response(
            jsonEncode({
              'token': 'synthetic-token',
              'profile': {'user_id': 'u', 'tenant_id': 't', 'role': 'doctor'}
            }),
            200);
      return http.Response(jsonEncode({'id': 'synthetic-session'}), 201);
    }));
    await client.login('doctor@example.invalid', 'synthetic-password');
    final s = await client.createSession('synthetic-template', 'DEMO',
        idempotencyKey: 'operation-one');
    expect(s['id'], 'synthetic-session');
    expect(requests.last.headers['Authorization'], 'Bearer synthetic-token');
    expect(requests.last.headers['Idempotency-Key'], 'operation-one');
    final body = jsonDecode(requests.last.body);
    expect(body['patient_ref'], 'DEMO');
    client.close();
  });
  test('chunk contract preserves revision and UTC timestamp', () {
    final c = TranscriptChunk(
        id: 'id',
        sequence: 2,
        revision: 3,
        isFinal: true,
        timestamp: DateTime.utc(2026, 11, 3, 12),
        text: 'synthetic');
    expect(c.toJson(), {
      'id': 'id',
      'sequence': 2,
      'revision': 3,
      'is_final': true,
      'timestamp': '2026-11-03T12:00:00.000Z',
      'text': 'synthetic'
    });
  });
  test('server errors propagate without automatic approval', () async {
    final client = WiseMedAIClient(Uri.parse('http://127.0.0.1:19120/'),
        client: MockClient((r) async => http.Response(
            '{"error":"explicit_current_revision_required"}', 409)));
    expect(() => client.approve('s', 1, confirmed: false),
        throwsA(isA<WiseMedAIException>()));
    client.close();
  });
}
