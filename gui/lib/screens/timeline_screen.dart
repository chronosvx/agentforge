import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';

class TimelineScreen extends StatefulWidget {
  const TimelineScreen({super.key, required this.api});

  final AgentForgeApi api;

  @override
  State<TimelineScreen> createState() => _TimelineScreenState();
}

class _TimelineScreenState extends State<TimelineScreen> {
  List<dynamic> _sessions = [];
  bool _loading = true;
  String? _error;

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
      final sessions = await widget.api.sessions();
      if (!mounted) return;
      setState(() {
        _sessions = sessions;
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
              Text('会话时间线', style: Theme.of(context).textTheme.headlineSmall),
              const Spacer(),
              IconButton(
                onPressed: _load,
                icon: const Icon(Icons.refresh),
                tooltip: '刷新',
              ),
            ],
          ),
          const SizedBox(height: 8),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : _error != null
                    ? Center(child: Text(_error!))
                    : _sessions.isEmpty
                        ? const Center(child: Text('暂无会话'))
                        : ListView.builder(
                            itemCount: _sessions.length,
                            itemBuilder: (context, index) {
                              final session =
                                  _sessions[index] as Map<String, dynamic>;
                              final title = session['title']
                                          ?.toString()
                                          .trim()
                                          .isNotEmpty ==
                                      true
                                  ? session['title'].toString()
                                  : '未命名会话';
                              final id = session['id']?.toString() ?? '';
                              final updated =
                                  session['updatedAt']?.toString() ?? '';
                              final messages =
                                  (session['messages'] as List?)?.length ?? 0;
                              return Card(
                                margin: const EdgeInsets.only(bottom: 8),
                                child: ListTile(
                                  leading: CircleAvatar(
                                    child: Text('${index + 1}'),
                                  ),
                                  title: Text(title),
                                  subtitle: Text(
                                    '$id\n更新于 $updated · $messages 条消息',
                                    maxLines: 3,
                                    overflow: TextOverflow.ellipsis,
                                  ),
                                  isThreeLine: true,
                                ),
                              );
                            },
                          ),
          ),
        ],
      ),
    );
  }
}
