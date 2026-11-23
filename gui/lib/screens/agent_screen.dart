import 'dart:async';

import 'package:flutter/material.dart';

import '../api/agentforge_api.dart';
import '../models/agent_event.dart';

/// 对话式 Agent 主界面。
///
/// 设计原则：像日常聊天软件一样，先输入任务、看结果；高级参数收进
/// “高级设置”，默认不打扰普通用户。
class AgentScreen extends StatefulWidget {
  const AgentScreen({super.key, required this.api});

  final AgentForgeApi api;

  @override
  State<AgentScreen> createState() => _AgentScreenState();
}

class _ChatEntry {
  const _ChatEntry.user(this.text)
      : event = null,
        fromUser = true;

  const _ChatEntry.agent(this.text, this.event) : fromUser = false;

  final bool fromUser;
  final String text;
  final AgentEvent? event;
}

class _AgentScreenState extends State<AgentScreen> {
  final _inputController = TextEditingController();

  // 高级设置（默认尽量贴近日常使用：静态规划、dry-run、自动批准）。
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
  bool _showAdvanced = false;
  bool _running = false;
  final List<_ChatEntry> _entries = [];
  final ScrollController _scrollController = ScrollController();
  StreamSubscription<Map<String, dynamic>>? _subscription;

  @override
  void dispose() {
    unawaited(_subscription?.cancel());
    _inputController.dispose();
    _workdirController.dispose();
    _providerController.dispose();
    _providerKeyController.dispose();
    _baseUrlController.dispose();
    _modelController.dispose();
    _testCmdController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final prompt = _inputController.text.trim();
    if (prompt.isEmpty || _running) return;
    _inputController.clear();
    setState(() {
      _entries.add(_ChatEntry.user(prompt));
      _running = true;
    });
    _scrollToBottom();

    try {
      await _subscription?.cancel();
      _subscription = null;
      if (!mounted) return;

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

      _subscription = widget.api.streamRunEvents(runId).listen(
        (data) {
          if (!mounted) return;
          final event = AgentEvent.fromJson(data);
          setState(() {
            _entries.add(_ChatEntry.agent(_eventTitle(event), event));
            if (data['type'] == 'done' || data['type'] == 'error') {
              _running = false;
            }
          });
          _scrollToBottom();
        },
        onError: (Object e) {
          if (!mounted) return;
          setState(() {
            _running = false;
            _entries.add(_ChatEntry.agent('运行出错：$e', null));
          });
          _scrollToBottom();
        },
        onDone: () {
          if (!mounted) return;
          setState(() => _running = false);
        },
      );
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _running = false;
        _entries.add(_ChatEntry.agent('无法启动任务：$e', null));
      });
      _scrollToBottom();
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scrollController.hasClients) return;
      _scrollController.animateTo(
        _scrollController.position.maxScrollExtent,
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOut,
      );
    });
  }

  String _eventTitle(AgentEvent ev) {
    return switch (ev.type) {
      'plan' => '已生成计划',
      'action' => ev.action?['status'] == 'started' ? '正在执行' : '动作完成',
      'test' => ev.testOk == true ? '测试通过' : '测试结果',
      'diff' => '产生了代码变更',
      'done' => '任务完成',
      'error' => '出错了',
      _ => '进度更新',
    };
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Expanded(
          child: _entries.isEmpty
              ? _buildEmptyState(context)
              : _buildChat(context),
        ),
        if (_showAdvanced) _buildAdvancedPanel(context),
        _buildInputBar(context),
      ],
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.auto_awesome,
                size: 64, color: theme.colorScheme.primary),
            const SizedBox(height: 16),
            Text('今天想让 Agent 帮你做什么？',
                style: theme.textTheme.titleLarge, textAlign: TextAlign.center),
            const SizedBox(height: 8),
            Text(
              '例如：给 main.go 增加 --version 参数',
              style: theme.textTheme.bodyMedium,
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            Wrap(
              spacing: 8,
              alignment: WrapAlignment.center,
              children: [
                ActionChip(
                  label: const Text('检查项目'),
                  onPressed: () {
                    _inputController.text = '帮我检查这个项目的结构和测试';
                    _send();
                  },
                ),
                ActionChip(
                  label: const Text('修复问题'),
                  onPressed: () {
                    _inputController.text = '帮我修复最近一次测试失败';
                    _send();
                  },
                ),
                ActionChip(
                  label: const Text('新增功能'),
                  onPressed: () {
                    _inputController.text = '帮我新增一个 --version 参数';
                    _send();
                  },
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildChat(BuildContext context) {
    return ListView.builder(
      controller: _scrollController,
      padding: const EdgeInsets.all(16),
      itemCount: _entries.length,
      itemBuilder: (context, index) {
        final entry = _entries[index];
        return entry.fromUser
            ? _buildUserBubble(context, entry.text)
            : _buildAgentBubble(context, entry);
      },
    );
  }

  Widget _buildUserBubble(BuildContext context, String text) {
    final theme = Theme.of(context);
    return Align(
      alignment: Alignment.centerRight,
      child: Container(
        margin: const EdgeInsets.only(bottom: 12, left: 48),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        decoration: BoxDecoration(
          color: theme.colorScheme.primaryContainer,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Text(text, style: theme.textTheme.bodyLarge),
      ),
    );
  }

  Widget _buildAgentBubble(BuildContext context, _ChatEntry entry) {
    final theme = Theme.of(context);
    final ev = entry.event;
    final detail = _eventDetail(entry.text, ev);
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 12, right: 48),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        decoration: BoxDecoration(
          color: theme.colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  _eventIcon(ev?.type),
                  size: 18,
                  color: theme.colorScheme.primary,
                ),
                const SizedBox(width: 8),
                Text(entry.text, style: theme.textTheme.titleSmall),
              ],
            ),
            if (detail.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(detail,
                  style: theme.textTheme.bodyMedium,
                  maxLines: 8,
                  overflow: TextOverflow.ellipsis),
            ],
          ],
        ),
      ),
    );
  }

  String _eventDetail(String title, AgentEvent? ev) {
    if (ev == null) return title;
    final actionDescription = ev.action?['description']?.toString() ?? '';
    final actionOutput = ev.action?['output']?.toString() ?? '';
    return (actionDescription.isNotEmpty ? actionDescription : null) ??
        (actionOutput.isNotEmpty ? actionOutput : null) ??
        (ev.testOutput.isNotEmpty ? ev.testOutput : null) ??
        (ev.error.isNotEmpty ? ev.error : null) ??
        (ev.diff.isNotEmpty ? ev.diff : null) ??
        '';
  }

  IconData _eventIcon(String? type) {
    return switch (type) {
      'plan' => Icons.list_alt,
      'action' => Icons.build,
      'test' => Icons.science,
      'diff' => Icons.difference,
      'done' => Icons.check_circle,
      'error' => Icons.error,
      _ => Icons.info,
    };
  }

  Widget _buildAdvancedPanel(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerLow,
        border: const Border(top: BorderSide(color: Colors.black12)),
      ),
      child: SingleChildScrollView(
        child: Column(
          children: [
            Row(
              children: [
                Text('高级设置', style: Theme.of(context).textTheme.titleSmall),
                const Spacer(),
                IconButton(
                  onPressed: () => setState(() => _showAdvanced = false),
                  icon: const Icon(Icons.expand_more),
                  tooltip: '收起高级设置',
                ),
              ],
            ),
            TextField(
              controller: _workdirController,
              decoration: const InputDecoration(
                labelText: '工作目录',
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                Expanded(
                  child: DropdownButtonFormField<String>(
                    initialValue: _planner,
                    decoration: const InputDecoration(
                      labelText: '规划器',
                      isDense: true,
                      border: OutlineInputBorder(),
                    ),
                    items: const [
                      DropdownMenuItem(value: 'static', child: Text('static')),
                      DropdownMenuItem(value: 'llm', child: Text('llm')),
                    ],
                    onChanged: (value) =>
                        setState(() => _planner = value ?? 'static'),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: TextField(
                    controller: _testCmdController,
                    decoration: const InputDecoration(
                      labelText: '测试命令',
                      isDense: true,
                      border: OutlineInputBorder(),
                    ),
                  ),
                ),
              ],
            ),
            if (_planner == 'llm') ...[
              const SizedBox(height: 8),
              TextField(
                controller: _providerController,
                decoration: const InputDecoration(
                  labelText: 'Provider',
                  hintText: 'deepseek / openai / ollama',
                  isDense: true,
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _providerKeyController,
                obscureText: true,
                decoration: const InputDecoration(
                  labelText: 'Provider Key',
                  isDense: true,
                  border: OutlineInputBorder(),
                ),
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _baseUrlController,
                      decoration: const InputDecoration(
                        labelText: 'Base URL（可选）',
                        isDense: true,
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: TextField(
                      controller: _modelController,
                      decoration: const InputDecoration(
                        labelText: 'Model（可选）',
                        isDense: true,
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                ],
              ),
            ],
            const SizedBox(height: 8),
            Wrap(
              spacing: 16,
              children: [
                Checkbox(
                  value: _repoMode,
                  onChanged: (v) => setState(() => _repoMode = v ?? false),
                ),
                const Text('Repo Mode'),
                Checkbox(
                  value: _autoApprove,
                  onChanged: (v) => setState(() => _autoApprove = v ?? false),
                ),
                const Text('自动批准'),
                Checkbox(
                  value: _dryRun,
                  onChanged: (v) => setState(() => _dryRun = v ?? false),
                ),
                const Text('Dry Run'),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildInputBar(BuildContext context) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            IconButton(
              onPressed: () => setState(() => _showAdvanced = !_showAdvanced),
              icon: Icon(_showAdvanced ? Icons.tune : Icons.tune_outlined),
              tooltip: '高级设置',
            ),
            const SizedBox(width: 4),
            Expanded(
              child: TextField(
                controller: _inputController,
                minLines: 1,
                maxLines: 5,
                textInputAction: TextInputAction.newline,
                onSubmitted: (_) => _send(),
                decoration: InputDecoration(
                  hintText: _running ? 'Agent 正在执行…' : '输入任务，按 Enter 发送',
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(24),
                  ),
                  contentPadding:
                      const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
                ),
              ),
            ),
            const SizedBox(width: 8),
            IconButton.filled(
              onPressed: _running ? null : _send,
              icon: _running
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.send),
              tooltip: '发送',
            ),
          ],
        ),
      ),
    );
  }
}
