import 'dart:async';
import 'dart:convert';
import 'dart:io';

/// AgentForge 本地 daemon 的轻量 HTTP/SSE 客户端。
///
/// 只依赖 Dart 标准库，便于桌面与后续移动端复用。
class AgentForgeApi {
  AgentForgeApi({
    required this.baseUrl,
    required this.token,
  }) {
    _client.connectionTimeout = const Duration(seconds: 10);
    _client.idleTimeout = const Duration(seconds: 30);
  }

  final String baseUrl;
  final String token;
  final HttpClient _client = HttpClient();

  void close() {
    _client.close();
  }

  Uri _uri(String path, [Map<String, String>? query]) {
    final uri = Uri.parse('$baseUrl$path');
    if (query == null || query.isEmpty) return uri;
    return uri.replace(queryParameters: query);
  }

  Map<String, String> get _headers => {'Authorization': 'Bearer $token'};

  Future<Map<String, dynamic>> health() async {
    final request = await _client.getUrl(_uri('/api/health'));
    request.headers.set('Authorization', _headers['Authorization']!);
    final response = await request.close();
    return _decodeObject(response, uri: request.uri);
  }

  Future<String> createRun(Map<String, dynamic> requestBody) async {
    final request = await _client.postUrl(_uri('/api/agent/runs'));
    request.headers.contentType = ContentType.json;
    request.headers.set('Authorization', _headers['Authorization']!);
    request.write(jsonEncode(requestBody));
    final response = await request.close();
    final body = await _decodeObject(response, uri: request.uri);
    return body['id'] as String;
  }

  /// 订阅一次 Agent 运行的 SSE 事件流。
  Stream<Map<String, dynamic>> streamRunEvents(String runId) async* {
    final request = await _client.getUrl(_uri('/api/agent/runs/$runId/events'));
    request.headers.set('Authorization', _headers['Authorization']!);
    final response = await request.close();
    if (response.statusCode != HttpStatus.ok) {
      throw HttpException('SSE failed: ${response.statusCode}',
          uri: request.uri);
    }
    var eventType = '';
    await for (final line
        in response.transform(utf8.decoder).transform(const LineSplitter())) {
      if (line.startsWith('event: ')) {
        eventType = line.substring('event: '.length).trim();
        continue;
      }
      if (!line.startsWith('data: ')) continue;
      final data = line.substring('data: '.length).trim();
      if (data.isEmpty) continue;
      final decoded = jsonDecode(data) as Map<String, dynamic>;
      if (eventType.isNotEmpty && decoded['type'] == null) {
        decoded['type'] = eventType;
      }
      eventType = '';
      yield decoded;
    }
  }

  Future<List<dynamic>> sessions() async {
    final request = await _client.getUrl(_uri('/api/sessions'));
    request.headers.set('Authorization', _headers['Authorization']!);
    final response = await request.close();
    final body = await _decodeObject(response, uri: request.uri);
    return (body['sessions'] as List?) ?? [];
  }

  Future<Map<String, dynamic>> memory({String? project}) async {
    final query = project == null ? null : {'project': project};
    final request = await _client.getUrl(_uri('/api/memory', query));
    request.headers.set('Authorization', _headers['Authorization']!);
    final response = await request.close();
    return _decodeObject(response, uri: request.uri);
  }

  Future<Map<String, dynamic>> _decodeObject(
    HttpClientResponse response, {
    Uri? uri,
  }) async {
    final text = await response.transform(utf8.decoder).join();
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw HttpException('HTTP ${response.statusCode}: $text', uri: uri);
    }
    return jsonDecode(text) as Map<String, dynamic>;
  }
}
