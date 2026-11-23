import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';

class StatusScreen extends StatefulWidget {
  const StatusScreen({super.key, required this.api});

  final AgentForgeApi api;

  @override
  State<StatusScreen> createState() => _StatusScreenState();
}

class _StatusScreenState extends State<StatusScreen> {
  Map<String, dynamic>? _health;
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final health = await widget.api.health();
      if (!mounted) return;
      setState(() {
        _health = health;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text('连接状态', style: Theme.of(context).textTheme.headlineSmall),
              const Spacer(),
              IconButton(
                onPressed: _load,
                icon: const Icon(Icons.refresh),
                tooltip: '刷新',
              ),
            ],
          ),
          const SizedBox(height: 12),
          if (_loading)
            const Center(child: CircularProgressIndicator())
          else if (_error != null)
            Card(
              child: ListTile(
                leading: const Icon(Icons.error, color: Colors.red),
                title: const Text('无法连接 daemon'),
                subtitle: Text(_error!),
              ),
            )
          else ...[
            Card(
              child: ListTile(
                leading: const Icon(
                  Icons.check_circle,
                  color: Colors.green,
                ),
                title: const Text('daemon 已连接'),
                subtitle: Text('状态: ${_health?['status'] ?? 'unknown'}'),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.info),
                title: const Text('版本'),
                subtitle: Text('${_health?['version'] ?? '-'}'),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.folder),
                title: const Text('数据目录'),
                subtitle: Text('${_health?['data_dir'] ?? '-'}'),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.schedule),
                title: const Text('启动时间'),
                subtitle: Text('${_health?['started_at'] ?? '-'}'),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
