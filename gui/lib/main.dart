import 'package:flutter/material.dart';

import 'api/agentforge_api.dart';
import 'api/settings_store.dart';
import 'screens/home_screen.dart';
import 'screens/settings_screen.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final settings = await SettingsStore().load();
  runApp(AgentForgeApp(
    initialBaseUrl: settings.baseUrl,
    initialToken: settings.token,
  ));
}

class AgentForgeApp extends StatefulWidget {
  const AgentForgeApp({
    super.key,
    required this.initialBaseUrl,
    required this.initialToken,
  });

  final String initialBaseUrl;
  final String initialToken;

  @override
  State<AgentForgeApp> createState() => _AgentForgeAppState();
}

class _AgentForgeAppState extends State<AgentForgeApp> {
  late String _baseUrl;
  late String _token;
  late AgentForgeApi _api;

  @override
  void initState() {
    super.initState();
    _baseUrl = widget.initialBaseUrl;
    _token = widget.initialToken;
    _api = AgentForgeApi(baseUrl: _baseUrl, token: _token);
  }

  Future<void> _saveSettings(String baseUrl, String token) async {
    await SettingsStore().save(baseUrl.trim(), token.trim());
    if (!mounted) return;
    _api.close();
    setState(() {
      _baseUrl = baseUrl.trim();
      _token = token.trim();
      _api = AgentForgeApi(baseUrl: _baseUrl, token: _token);
    });
  }

  @override
  void dispose() {
    _api.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'AgentForge',
      theme: ThemeData(
        colorSchemeSeed: Colors.indigo,
        brightness: Brightness.dark,
        useMaterial3: true,
      ),
      home: HomeScreen(
        api: _api,
        onOpenSettings: () async {
          await Navigator.of(context).push(MaterialPageRoute(
            builder: (_) => SettingsScreen(
              baseUrl: _baseUrl,
              token: _token,
              onSave: _saveSettings,
            ),
          ));
        },
      ),
    );
  }
}
