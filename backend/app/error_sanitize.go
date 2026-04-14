package app

import "strings"

// sanitizeError 对错误消息做脱敏与长度截断，避免泄漏敏感值。
func sanitizeError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	return SanitizeErrorForTest(msg, secrets...)
}

// SanitizeErrorForTest 为公开的脱敏函数，供测试与外部调用验证脱敏行为。
//
// 输入：
// - msg: 原始消息字符串。
// - secrets: 需要替换的敏感值列表。
//
// 输出：
// - string: 脱敏并截断后的消息。
func SanitizeErrorForTest(msg string, secrets ...string) string {
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
