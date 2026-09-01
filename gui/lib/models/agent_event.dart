/// Agent 运行事件的轻量模型。
class AgentEvent {
  const AgentEvent({
    required this.type,
    this.actions = const [],
    this.action,
    this.testOutput = '',
    this.testOk,
    this.diff = '',
    this.error = '',
  });

  final String type;
  final List<Map<String, dynamic>> actions;
  final Map<String, dynamic>? action;
  final String testOutput;
  final bool? testOk;
  final String diff;
  final String error;

  factory AgentEvent.fromJson(Map<String, dynamic> json) {
    return AgentEvent(
      type: json['type'] as String? ?? '',
      actions: (json['actions'] as List? ?? const [])
          .map((e) => e as Map<String, dynamic>)
          .toList(),
      action: json['action'] as Map<String, dynamic>?,
      testOutput: json['test_output'] as String? ?? '',
      testOk: json['test_ok'] as bool?,
      diff: json['diff'] as String? ?? '',
      error: json['error'] as String? ?? '',
    );
  }
}
