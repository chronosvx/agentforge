import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';

class MemoryScreen extends StatefulWidget {
  const MemoryScreen({super.key, required this.api});

  final AgentForgeApi api;

  @override
  State<MemoryScreen> createState() => _MemoryScreenState();
}

class _MemoryScreenState extends State<MemoryScreen> {
  List<String> _projects = [];
  List<Map<String, dynamic>> _entries = [];
  String _selectedProject = '';
  bool _loading = true;
  String? _error;
  int _loadGeneration = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final generation = ++_loadGeneration;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final data = await widget.api.memory(
        project: _selectedProject.isEmpty ? null : _selectedProject,
      );
      if (!mounted || generation != _loadGeneration) return;
      setState(() {
        final projects =
            (data['projects'] as List? ?? []).map((e) => e.toString()).toList();
        if (!projects.contains(_selectedProject)) {
          _selectedProject = '';
        }
        _projects = projects;
        _entries = (data['entries'] as List? ?? [])
            .map((e) => e as Map<String, dynamic>)
            .toList();
        _loading = false;
      });
    } catch (e) {
      if (!mounted || generation != _loadGeneration) return;
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
          Text('项目记忆', style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 12),
          Row(
            children: [
              DropdownButton<String>(
                value: _selectedProject,
                items: [
                  const DropdownMenuItem(value: '', child: Text('全部项目')),
                  for (final p in _projects)
                    DropdownMenuItem(value: p, child: Text(p)),
                ],
                onChanged: (value) {
                  setState(() => _selectedProject = value ?? '');
                  _load();
                },
              ),
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
                    : _entries.isEmpty
                        ? const Center(child: Text('暂无记忆'))
                        : ListView.builder(
                            itemCount: _entries.length,
                            itemBuilder: (context, index) {
                              final entry = _entries[index];
                              return Card(
                                margin: const EdgeInsets.only(bottom: 8),
                                child: ListTile(
                                  leading: const Icon(Icons.bookmark),
                                  title: Text(
                                      '${entry['project'] ?? ''} / ${entry['key'] ?? ''}'),
                                  subtitle: Text(
                                    entry['content']?.toString() ?? '',
                                    maxLines: 4,
                                    overflow: TextOverflow.ellipsis,
                                  ),
                                  trailing: Text(
                                    entry['updated_at']
                                            ?.toString()
                                            .split('T')
                                            .first ??
                                        '',
                                    style:
                                        Theme.of(context).textTheme.bodySmall,
                                  ),
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
