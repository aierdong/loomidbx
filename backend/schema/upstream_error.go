package schema

import (
	"context"
	"errors"
	"strings"
)

const (
	// UpstreamCodeDeadlineExceeded 表示在超时边界内未完成（包含上游超时与本地超时）。
	UpstreamCodeDeadlineExceeded = "DEADLINE_EXCEEDED"
	// UpstreamCodeUpstreamUnavailable 表示目标数据库不可达或连接被拒绝。
	UpstreamCodeUpstreamUnavailable = "UPSTREAM_UNAVAILABLE"
	// UpstreamCodeAuthFailed 表示认证失败（用户名/密码错误等）。
	UpstreamCodeAuthFailed = "AUTH_FAILED"
	// UpstreamCodePermissionDenied 表示上游数据库权限不足（无读取元数据权限等）。
	UpstreamCodePermissionDenied = "PERMISSION_DENIED"
)

// UpstreamClassifiedError 表示已按设计映射后的上游错误。
type UpstreamClassifiedError struct {
	// Code 为稳定错误码（UPSTREAM_UNAVAILABLE / AUTH_FAILED / PERMISSION_DENIED / DEADLINE_EXCEEDED 等）。
	Code string

	// Message 为可读错误信息，已脱敏且不包含敏感值。
	Message string

	// Details 为可选上下文信息，禁止包含敏感值。
	Details map[string]string
}

// ClassifyUpstreamError 将任意上游错误归类为稳定错误码并做脱敏处理。
//
// 输入：
// - ctx: 用于判定是否超时/取消。
// - err: 原始错误。
// - secrets: 需要脱敏替换的敏感值列表（如明文密码、token、DSN 片段）。
//
// 输出：
// - *UpstreamClassifiedError: 归类后的错误对象；err=nil 时返回 nil。
func ClassifyUpstreamError(ctx context.Context, err error, secrets ...string) *UpstreamClassifiedError {
	if err == nil {
		return nil
	}

	code := classifyUpstreamCode(ctx, err)
	msg := sanitizeAndTruncate(err.Error(), secrets...)

	// 额外兜底：避免把 URL/DSN 中的 password=xxx 形式直接带出
	msg = sanitizeCommonCredentialPatterns(msg)

	return &UpstreamClassifiedError{
		Code:    code,
		Message: msg,
	}
}

func classifyUpstreamCode(ctx context.Context, err error) string {
	if ctx != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return UpstreamCodeDeadlineExceeded
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			// 取消并非上游错误，但在扫描任务域仍应稳定输出为前置条件失败；
			// 2.1 仅要求上游错误映射，先用 DEADLINE_EXCEEDED 语义占位。
			return UpstreamCodeDeadlineExceeded
		}
	}

	msg := strings.ToLower(err.Error())

	// 认证失败
	if containsAny(msg,
		"access denied",
		"authentication failed",
		"invalid password",
		"login failed",
		"password authentication failed",
		"no such user",
	) {
		return UpstreamCodeAuthFailed
	}

	// 权限不足
	if containsAny(msg,
		"permission denied",
		"insufficient privilege",
		"not authorized",
		"access is denied",
	) {
		return UpstreamCodePermissionDenied
	}

	// 连接不可达
	if containsAny(msg,
		"connection refused",
		"no route to host",
		"network",
		"dial",
		"connect",
		"unreachable",
		"reset by peer",
		"broken pipe",
		"server closed the connection",
	) {
		return UpstreamCodeUpstreamUnavailable
	}

	// 其他默认归为上游不可达（避免泄漏驱动内部错误类别）
	return UpstreamCodeUpstreamUnavailable
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func sanitizeAndTruncate(msg string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}

func sanitizeCommonCredentialPatterns(msg string) string {
	// 极简兜底：把 password=xxx、pwd=xxx、pass=xxx 做替换，避免未显式传入 secrets 的情况。
	//
	// 注意：替换后仍可能残留关键字（例如 password=*** 仍包含 "password="），
	// 因此必须按“扫描位置前进”的方式处理，避免死循环。
	repls := []string{"password=", "pwd=", "pass="}
	out := msg
	pos := 0
	for pos < len(out) {
		lower := strings.ToLower(out[pos:])
		next := -1
		var matched string
		for _, key := range repls {
			idx := strings.Index(lower, key)
			if idx < 0 {
				continue
			}
			if next < 0 || idx < next {
				next = idx
				matched = key
			}
		}
		if next < 0 {
			break
		}

		abs := pos + next
		start := abs + len(matched)
		end := start
		for end < len(out) {
			ch := out[end]
			if ch == '&' || ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == ';' {
				break
			}
			end++
		}
		if end > start {
			out = out[:start] + "***" + out[end:]
			// 从替换点之后继续扫描，避免在同一位置反复匹配。
			pos = start + len("***")
			continue
		}

		// 没有可替换的值，跳过该关键字，避免卡死。
		pos = start
	}
	return out
}

