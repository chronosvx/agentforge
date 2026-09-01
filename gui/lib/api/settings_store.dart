import 'dart:convert';
import 'dart:io';

/// 极简本地设置存储。
///
/// 为避免在 macOS 首版引入 CocoaPods/插件依赖，直接读写用户主目录下的
/// `~/.agentforge/gui.json`。后续移动端再按平台替换为 path_provider。
class SettingsStore {
  static const String _defaultBaseUrl = 'http://127.0.0.1:48321';

  Future<({String baseUrl, String token})> load() async {
    try {
      final home = Platform.environment['HOME'] ?? '.';
      final file = File('$home/.agentforge/gui.json');
      if (!await file.exists()) {
        return (baseUrl: _defaultBaseUrl, token: '');
      }
      final data =
          jsonDecode(await file.readAsString()) as Map<String, dynamic>;
      return (
        baseUrl: data['server_url']?.toString() ?? _defaultBaseUrl,
        token: data['server_token']?.toString() ?? '',
      );
    } catch (_) {
      return (baseUrl: _defaultBaseUrl, token: '');
    }
  }

  Future<void> save(String baseUrl, String token) async {
    final home = Platform.environment['HOME'] ?? '.';
    final dir = Directory('$home/.agentforge');
    await dir.create(recursive: true);
    final file = File('$home/.agentforge/gui.json');
    await file.writeAsString(
      jsonEncode({
        'server_url': baseUrl,
        'server_token': token,
      }),
      flush: true,
    );
    // 权限尽量收紧；Windows 上该调用可能不支持，忽略错误。
    try {
      await Process.run('chmod', ['600', file.path]);
    } catch (_) {}
  }
}
