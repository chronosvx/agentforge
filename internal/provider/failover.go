package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// FailoverRouter 包装 Router 与多个 Provider，在调用失败时自动切换到下一个可用厂商。
type FailoverRouter struct {
	Router *Router
}

// NewFailoverRouter 创建一个故障转移路由器。
//
// 参数兼容两种常见用法：
//   - NewFailoverRouter(router) 使用已有 Router；
//   - NewFailoverRouter(p1, p2, ...) 直接用多个 Provider 创建，优先级均为 0。
//
// 如果传入的 router 为 nil，会创建一个新的 Router；额外传入的 providers 会以优先级 0 添加到该 Router。
func NewFailoverRouter(args ...any) *FailoverRouter {
	var router *Router
	var providers []Provider
	for _, arg := range args {
		switch v := arg.(type) {
		case *Router:
			router = v
		case Provider:
			providers = append(providers, v)
		}
	}
	if router == nil {
		router = NewRouter()
	}
	for _, p := range providers {
		if p != nil {
			router.Add(p, 0)
		}
	}
	return &FailoverRouter{Router: router}
}

// NewFailoverRouterWithProviders 直接用多个 Provider 创建故障转移路由器，所有 Provider 优先级为 0。
func NewFailoverRouterWithProviders(providers ...Provider) *FailoverRouter {
	router := NewRouter()
	for _, p := range providers {
		if p != nil {
			router.Add(p, 0)
		}
	}
	return &FailoverRouter{Router: router}
}

// AggregateError 聚合多个厂商失败原因。
type AggregateError struct {
	Errors []error
}

func (e *AggregateError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "provider: all providers failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return "provider: all providers failed"
	}
	return "provider: all providers failed: " + strings.Join(parts, "; ")
}

func (e *AggregateError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Errors
}

func (e *AggregateError) Is(target error) bool {
	if e == nil {
		return false
	}
	for _, err := range e.Errors {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// shouldFailover 判断一个错误是否应触发切换到下一个 Provider。
// 现有 ProviderError 本身代表厂商级错误，因此无论 Retryable 是否为 true 都切换；
// 同时兼容 IsRetryable 识别的可重试错误。
func shouldFailover(err error) bool {
	if err == nil {
		return false
	}
	if IsRetryable(err) {
		return true
	}
	var pe *ProviderError
	return errors.As(err, &pe)
}

// Chat 依次尝试 Provider，成功即返回；遇到可切换错误时切换到下一个。
// 所有 Provider 均失败时返回 *AggregateError。
func (f *FailoverRouter) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if f == nil || f.Router == nil {
		return nil, ErrNoProvider
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	var errs []error
	for {
		p, err := f.Router.Next(ctx)
		if err != nil {
			if errors.Is(err, ErrNoProvider) {
				if len(errs) > 0 {
					return nil, &AggregateError{Errors: errs}
				}
				return nil, ErrNoProvider
			}
			return nil, err
		}

		resp, err := p.Chat(ctx, req)
		if err == nil {
			return resp, nil
		}

		errs = append(errs, err)
		f.Router.MarkFailed(p)
		if !shouldFailover(err) {
			return nil, fmt.Errorf("provider %s: %w", p.Name(), err)
		}
	}
}

// StreamChat 与 Chat 类似，但对流式接口做故障转移。流一旦成功建立（返回非 nil channel），
// 就立即返回给调用方，后续流内错误不再切换厂商。
func (f *FailoverRouter) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if f == nil || f.Router == nil {
		return nil, ErrNoProvider
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	var errs []error
	for {
		p, err := f.Router.Next(ctx)
		if err != nil {
			if errors.Is(err, ErrNoProvider) {
				if len(errs) > 0 {
					return nil, &AggregateError{Errors: errs}
				}
				return nil, ErrNoProvider
			}
			return nil, err
		}

		stream, err := p.StreamChat(ctx, req)
		if err == nil {
			return stream, nil
		}

		errs = append(errs, err)
		f.Router.MarkFailed(p)
		if !shouldFailover(err) {
			return nil, fmt.Errorf("provider %s: %w", p.Name(), err)
		}
	}
}
