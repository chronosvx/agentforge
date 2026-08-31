// Package plugin 提供可扩展的插件运行时基础。
//
// 当前版本聚焦于插件发现、注册、执行与事件钩子：
//   - 外部插件：目录中的 *.dsh-plugin.json 清单 + 相邻可执行文件；
//   - 内置插件：通过 RegisterBuiltin 注册的纯 Go 函数；
//   - 事件钩子：内置插件通过 RegisterHook 订阅，外部插件通过 manifest 的 hooks 声明。
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultTimeout 是外部插件命令的默认执行超时。
	DefaultTimeout = 30 * time.Second
	// ManifestSuffix 是插件清单文件的后缀。
	ManifestSuffix = ".dsh-plugin.json"
)

// HookName 是插件可订阅的事件钩子名称。
type HookName string

const (
	// HookOnMessage 在聊天消息产生时触发。
	HookOnMessage HookName = "on_message"
	// HookOnToolResult 在工具结果产生时触发。
	HookOnToolResult HookName = "on_tool_result"
	// HookOnDiff 在 diff 结果产生时触发。
	HookOnDiff HookName = "on_diff"
	// HookOnCost 在成本信息产生时触发。
	HookOnCost HookName = "on_cost"
	// HookOnSessionSwitch 在会话切换时触发。
	HookOnSessionSwitch HookName = "on_session_switch"
)

// ErrCommandNotFound 表示 RunCommand 未找到对应插件或命令别名。
var ErrCommandNotFound = errors.New("plugin: command not found")

// ErrTimeout 表示外部插件执行超时。
var ErrTimeout = errors.New("plugin: command timed out")

// Manifest 描述一个插件的元信息。
type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Commands    []string `json:"commands,omitempty"`
	Hooks       []string `json:"hooks,omitempty"`
}

// Plugin 是一个已加载的插件。
// Executable 为空时表示内置插件，执行体为 builtin 函数。
type Plugin struct {
	Manifest   Manifest
	Executable string // 外部可执行文件路径；内置插件为空
	builtin    func(args []string) (string, error)
}

// IsBuiltin 报告插件是否为纯 Go 注册的内置插件。
func (p Plugin) IsBuiltin() bool {
	return p.builtin != nil
}

// hookRegistration 记录一次通过 RegisterHook 注册的事件钩子。
type hookRegistration struct {
	pluginName string
	hook       HookName
	fn         func(event map[string]any) error
}

// Manager 负责管理插件列表并执行插件命令与事件钩子。
type Manager struct {
	workDir    string
	workDirSet bool
	timeout    time.Duration

	plugins map[string]*Plugin
	aliases map[string]string // 命令名 -> 插件名
	order   []string
	hooks   []hookRegistration
}

// New 返回使用当前工作目录作为外部插件执行目录的 Manager。
func New() *Manager {
	return NewManager("")
}

// NewManager 返回一个 Manager。
// 可选的 workDir 是外部插件执行时的工作目录；为空时使用当前进程工作目录，
// 若之后调用 LoadDir 且仍未设置，则自动使用被扫描的插件目录。
func NewManager(workDir ...string) *Manager {
	var dir string
	if len(workDir) > 0 {
		dir = workDir[0]
	}
	return &Manager{
		workDir:    dir,
		workDirSet: dir != "",
		timeout:    DefaultTimeout,
		plugins:    make(map[string]*Plugin),
		aliases:    make(map[string]string),
	}
}

// SetTimeout 设置外部插件命令的超时时间。
func (m *Manager) SetTimeout(d time.Duration) {
	m.init()
	if d > 0 {
		m.timeout = d
	}
}

// RegisterBuiltin 注册一个纯 Go 内置插件。
// 同名内置插件可被替换；同名外部插件不可被覆盖。
func (m *Manager) RegisterBuiltin(name string, fn func(args []string) (string, error)) error {
	m.init()
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("plugin: builtin name cannot be empty")
	}
	if fn == nil {
		return fmt.Errorf("plugin: builtin %q function cannot be nil", name)
	}
	if existing, ok := m.plugins[name]; ok && existing.builtin == nil {
		return fmt.Errorf("plugin: %q already registered as external plugin", name)
	}
	if _, ok := m.aliases[name]; !ok {
		m.aliases[name] = name
	}
	m.plugins[name] = &Plugin{
		Manifest: Manifest{
			Name:        name,
			Description: "built-in plugin",
			Commands:    []string{name},
		},
		builtin: fn,
	}
	if !containsString(m.order, name) {
		m.order = append(m.order, name)
	}
	return nil
}

// RegisterHook 为指定插件注册一个事件钩子。
// 回调会在 Emit 触发对应事件时被调用；返回的错误会被 Emit 收集但不会中断其他钩子。
func (m *Manager) RegisterHook(pluginName string, hook HookName, fn func(event map[string]any) error) error {
	m.init()
	pluginName = strings.TrimSpace(pluginName)
	if pluginName == "" {
		return fmt.Errorf("plugin: hook plugin name cannot be empty")
	}
	if hook == "" {
		return fmt.Errorf("plugin: hook name cannot be empty")
	}
	if fn == nil {
		return fmt.Errorf("plugin: hook %q for %q cannot be nil", hook, pluginName)
	}
	m.hooks = append(m.hooks, hookRegistration{
		pluginName: pluginName,
		hook:       hook,
		fn:         fn,
	})
	return nil
}

// Register 是 RegisterBuiltin 的别名。
func (m *Manager) Register(name string, fn func(args []string) (string, error)) error {
	return m.RegisterBuiltin(name, fn)
}

// AddBuiltin 是 RegisterBuiltin 的别名。
func (m *Manager) AddBuiltin(name string, fn func(args []string) (string, error)) error {
	return m.RegisterBuiltin(name, fn)
}

// LoadDir 扫描目录中的 *.dsh-plugin.json 清单并加载相邻可执行文件。
// 目录不存在时视为空目录，不返回错误。
func (m *Manager) LoadDir(dir string) error {
	m.init()
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("plugin: stat plugin dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugin: %s is not a directory", dir)
	}

	if !m.workDirSet {
		m.workDir = dir
	}

	manifests, err := findManifests(dir)
	if err != nil {
		return err
	}

	// 先解析并校验整批清单，避免半加载。
	loaded := make([]*Plugin, 0, len(manifests))
	seenNames := make(map[string]string, len(manifests))
	seenAliases := make(map[string]string, len(manifests))
	for _, path := range manifests {
		p, err := loadManifest(path)
		if err != nil {
			return err
		}
		name := p.Manifest.Name
		if prev, ok := seenNames[name]; ok {
			return fmt.Errorf("plugin: duplicate plugin name %q in %s and %s", name, prev, path)
		}
		seenNames[name] = path
		if _, exists := m.plugins[name]; exists {
			return fmt.Errorf("plugin: plugin %q already exists", name)
		}
		for _, cmd := range p.Manifest.Commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" {
				continue
			}
			if prev, ok := seenAliases[cmd]; ok {
				return fmt.Errorf("plugin: command %q conflicts between %q and %q", cmd, prev, name)
			}
			if prev, ok := m.aliases[cmd]; ok {
				return fmt.Errorf("plugin: command %q already registered by %q", cmd, prev)
			}
			seenAliases[cmd] = name
		}
		loaded = append(loaded, p)
	}

	for _, p := range loaded {
		m.addPlugin(p)
	}
	return nil
}

// List 返回当前已加载的插件列表，按名称排序。
func (m *Manager) List() []Plugin {
	m.init()
	out := make([]Plugin, 0, len(m.plugins))
	for _, name := range m.order {
		if p, ok := m.plugins[name]; ok {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.Name < out[j].Manifest.Name
	})
	return out
}

// RunCommand 执行指定插件命令。
// name 可以是插件名，也可以是 Manifest.Commands 中登记的别名。
func (m *Manager) RunCommand(name string, args []string) (string, error) {
	m.init()
	if args == nil {
		args = []string{}
	}
	pluginName, ok := m.aliases[strings.TrimSpace(name)]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}
	p, ok := m.plugins[pluginName]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}
	if p.builtin != nil {
		return p.builtin(args)
	}
	return m.runExternal(p, args)
}

// Emit 触发一个事件钩子，调用所有已注册钩子以及声明了该事件的外部插件。
// 错误会被收集并返回，但不会中断后续钩子的执行。
func (m *Manager) Emit(eventName HookName, event map[string]any) []error {
	m.init()
	if eventName == "" {
		return []error{fmt.Errorf("plugin: hook name cannot be empty")}
	}

	var errs []error
	for _, h := range m.hooks {
		if h.hook != eventName {
			continue
		}
		if err := h.fn(event); err != nil {
			errs = append(errs, fmt.Errorf("plugin: hook %q (plugin %q): %w", eventName, h.pluginName, err))
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		errs = append(errs, fmt.Errorf("plugin: marshal event for hook %q: %w", eventName, err))
		return errs
	}
	for _, name := range m.order {
		p := m.plugins[name]
		if p == nil || p.builtin != nil {
			continue
		}
		if !containsString(p.Manifest.Hooks, string(eventName)) {
			continue
		}
		if _, err := m.runExternal(p, []string{"--hook", string(eventName), string(data)}); err != nil {
			errs = append(errs, fmt.Errorf("plugin: external hook %q (plugin %q): %w", eventName, name, err))
		}
	}
	return errs
}

func (m *Manager) init() {
	if m.plugins == nil {
		m.plugins = make(map[string]*Plugin)
	}
	if m.aliases == nil {
		m.aliases = make(map[string]string)
	}
	if m.timeout <= 0 {
		m.timeout = DefaultTimeout
	}
}

func (m *Manager) addPlugin(p *Plugin) {
	name := p.Manifest.Name
	m.plugins[name] = p
	m.order = append(m.order, name)
	m.aliases[name] = name
	for _, cmd := range p.Manifest.Commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		m.aliases[cmd] = name
	}
}

func (m *Manager) runExternal(p *Plugin, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.Executable, args...)
	cmd.Dir = m.workDir
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return strings.TrimSpace(string(out)), fmt.Errorf("%w: command %q timed out after %s: %w", ErrTimeout, p.Manifest.Name, m.timeout, context.DeadlineExceeded)
	}
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("plugin: run %q: %w", p.Manifest.Name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func findManifests(dir string) ([]string, error) {
	var manifests []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ManifestSuffix) {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plugin: walk plugin dir: %w", err)
	}
	sort.Strings(manifests)
	return manifests, nil
}

type fileManifest struct {
	Manifest
	Executable string `json:"executable"`
}

func loadManifest(path string) (*Plugin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read manifest %s: %w", path, err)
	}
	var fm fileManifest
	if err := json.Unmarshal(data, &fm); err != nil {
		return nil, fmt.Errorf("plugin: parse manifest %s: %w", path, err)
	}
	fm.Name = strings.TrimSpace(fm.Name)
	if fm.Name == "" {
		return nil, fmt.Errorf("plugin: manifest %s missing name", path)
	}

	execPath := strings.TrimSpace(fm.Executable)
	if execPath == "" {
		base := strings.TrimSuffix(filepath.Base(path), ManifestSuffix)
		execPath = filepath.Join(filepath.Dir(path), base)
	} else if !filepath.IsAbs(execPath) {
		execPath = filepath.Join(filepath.Dir(path), execPath)
	}
	execPath = filepath.Clean(execPath)

	if _, err := os.Stat(execPath); err != nil {
		return nil, fmt.Errorf("plugin: manifest %s executable %s: %w", path, execPath, err)
	}

	return &Plugin{
		Manifest:   fm.Manifest,
		Executable: execPath,
	}, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
