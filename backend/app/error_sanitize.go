package app

import "strings"

// sanitizeError 对错误消息做脱敏与长度截断，避免泄漏敏感值。
func sanitizeError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
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
