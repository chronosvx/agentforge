import 'dart:async';

import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';
import '../models/agent_event.dart';

class AgentScreen extends StatefulWidget {
  const AgentScreen({super.key, required this.api});

  final AgentForgeApi api;

  @override
  State<AgentScreen> createState() => _AgentScreenState();
}

class _AgentScreenState extends State<AgentScreen> {
  final _promptController = TextEditingController();
  final _workdirController = TextEditingController(text: '.');
  final _providerController = TextEditingController();
  final _providerKeyController = TextEditingController();
  final _baseUrlController = TextEditingController();
  final _modelController = TextEditingController();
  final _testCmdController = TextEditingController(text: 'go test ./...');

  String _planner = 'static';
  bool _repoMode = false;
  bool _autoApprove = true;
  bool _dryRun = true;
  bool _running = false;
  String _runId = '';
  String? _error;
  final List<AgentEvent> _events = [];
  StreamSubscription<Map<String, dynamic>>? _subscription;

  @override
  void dispose() {
    unawaited(_subscription?.cancel());
    _promptController.dispose();
    _workdirController.dispose();
    _providerController.dispose();
    _providerKeyController.dispose();
    _baseUrlController.dispose();
    _modelController.dispose();
    _testCmdController.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    final prompt = _promptController.text.trim();
    if (prompt.isEmpty) return;
    setState(() {
      _running = true;
      _error = null;
      _runId = '';
    });

    try {
      await _subscription?.cancel();
      _subscription = null;
      if (!mounted) return;
      setState(() {
        _events.clear();
        _error = null;
        _running = true;
      });

      final runId = await widget.api.createRun({
        'prompt': prompt,
        'work_dir': _workdirController.text.trim().isEmpty
            ? '.'
            : _workdirController.text.trim(),
        'repo_mode': _repoMode,
        'planner': _planner,
        'provider': _providerController.text.trim(),
        'provider_key': _providerKeyController.text.trim(),
        'base_url': _baseUrlController.text.trim(),
        'model': _modelController.text.trim(),
        'auto_approve': _autoApprove,
        'dry_run': _dryRun,
        'test_cmd': _testCmdController.text.trim(),
      });
      if (!mounted) return;
      setState(() => _runId = runId);

      _subscription = widget.api.streamRunEvents(runId).listen(
        (data) {
          if (!mounted) return;
          setState(() {
            _events.add(AgentEvent.fromJson(data));
            if (data['type'] == 'done') {
              _running = false;
            }
            if (data['type'] == 'error') {
              _error = data['error'] as String? ?? '运行失败';
              _running = false;
            }
          });
        },
        onError: (Object e) {
          if (!mounted) return;
          setState(() {
            _error = e.toString();
            _running = false;
          });
        },
        onDone: () {
          if (!mounted) return;
          setState(() => _running = false);
        },
      );
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _running = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            flex: 3,
            child: _buildForm(context),
          ),
          const SizedBox(width: 16),
          Expanded(
            flex: 4,
            child: _buildOutput(context),
          ),
        ],
      ),
    );
  }

  Widget _buildForm(BuildContext context) {
    return ListView(
      children: [
        Text('Agent 运行', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 12),
        TextField(
          controller: _promptController,
          maxLines: 4,
          decoration: const InputDecoration(
            labelText: '任务描述',
            hintText: '例如：给 main.go 增加 --version 参数',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _workdirController,
          decoration: const InputDecoration(
            labelText: '工作目录',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<String>(
          initialValue: _planner,
          decoration: const InputDecoration(
            labelText: '规划器',
            border: OutlineInputBorder(),
          ),
          items: const [
            DropdownMenuItem(value: 'static', child: Text('static')),
            DropdownMenuItem(value: 'llm', child: Text('llm')),
          ],
          onChanged: (value) => setState(() => _planner = value ?? 'static'),
        ),
        const SizedBox(height: 12),
        if (_planner == 'llm') ...[
          TextField(
            controller: _providerController,
            decoration: const InputDecoration(
              labelText: 'Provider',
              hintText: 'deepseek / openai / ollama',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _providerKeyController,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Provider Key',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _baseUrlController,
            decoration: const InputDecoration(
              labelText: 'Base URL（可选）',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _modelController,
            decoration: const InputDecoration(
              labelText: 'Model（可选）',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
        ],
        TextField(
          controller: _testCmdController,
          decoration: const InputDecoration(
            labelText: '测试命令',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 8),
        CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('Repo Mode'),
          value: _repoMode,
          onChanged: (v) => setState(() => _repoMode = v ?? false),
        ),
        CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('自动批准'),
          value: _autoApprove,
          onChanged: (v) => setState(() => _autoApprove = v ?? false),
        ),
        CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('Dry Run（只预览不实际执行）'),
          value: _dryRun,
          onChanged: (v) => setState(() => _dryRun = v ?? false),
        ),
        const SizedBox(height: 8),
        FilledButton.icon(
          onPressed: _running ? null : _start,
          icon: _running
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.play_arrow),
          label: Text(_running ? '运行中…' : '开始运行'),
        ),
        if (_runId.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text('Run ID: $_runId',
                style: Theme.of(context).textTheme.bodySmall),
          ),
      ],
    );
  }

  Widget _buildOutput(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('输出', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            if (_error != null)
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.errorContainer,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(_error!),
              ),
            Expanded(
              child: _events.isEmpty
                  ? const Center(child: Text('等待任务开始…'))
                  : ListView(
                      children: [
                        for (final ev in _events) _buildEvent(context, ev),
                      ],
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildEvent(BuildContext context, AgentEvent ev) {
    final icon = switch (ev.type) {
      'plan' => Icons.list_alt,
      'action' => Icons.build,
      'test' => Icons.science,
      'diff' => Icons.difference,
      'done' => Icons.check_circle,
      'error' => Icons.error,
      _ => Icons.info,
    };
    final title = switch (ev.type) {
      'plan' => '计划',
      'action' => ev.action?['status'] == 'started' ? '执行开始' : '动作完成',
      'test' => '测试',
      'diff' => 'Diff',
      'done' => '完成',
      'error' => '错误',
      _ => ev.type,
    };
    final actionDescription = ev.action?['description']?.toString() ?? '';
    final actionOutput = ev.action?['output']?.toString() ?? '';
    final detail = (actionDescription.isNotEmpty ? actionDescription : null) ??
        (actionOutput.isNotEmpty ? actionOutput : null) ??
        (ev.testOutput.isNotEmpty ? ev.testOutput : null) ??
        (ev.error.isNotEmpty ? ev.error : null) ??
        (ev.diff.isNotEmpty ? ev.diff : null) ??
        '';

    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: Icon(icon),
        title: Text(title),
        subtitle: detail.isEmpty ? null : Text(detail, maxLines: 6),
      ),
    );
  }
}
