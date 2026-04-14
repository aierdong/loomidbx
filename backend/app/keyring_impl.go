//go:build windows || darwin || linux

package app

import (
	keyring "github.com/zalando/go-keyring"
)

// isKeyringAvailable 检测密钥环是否可用。
//
// 通过尝试读取一个不存在的条目来探测：
// - 返回 keyring.ErrNotFound 表示密钥环可用
// - 返回其他错误表示密钥环不可用或访问被拒绝
func isKeyringAvailable(service string) error {
	// 尝试读取一个探测条目，用于检测密钥环是否可达
	_, err := keyring.Get(service, "__loomidbx_probe__")
	if err == nil {
		// 探测条目意外存在，清理后返回可用
		_ = keyring.Delete(service, "__loomidbx_probe__")
		return nil
	}

	// ErrNotFound 表示密钥环服务可用，只是条目不存在
	if isKeyringNotFound(err) {
		return nil
	}

	// 其他错误需要进一步分类
	return wrapKeyringError(err)
}

// getKeyringSecret 从密钥环读取凭据。
func getKeyringSecret(service, ref string) (string, error) {
	secret, err := keyring.Get(service, ref)
	if err != nil {
		if isKeyringNotFound(err) {
			// 条目不存在视为不可用（凭据缺失）
			return "", ErrKeyringUnavailable
		}
		return "", wrapKeyringError(err)
	}
	return secret, nil
}

// setKeyringSecret 将凭据存入密钥环。
func setKeyringSecret(service, ref, secret string) error {
	err := keyring.Set(service, ref, secret)
	if err != nil {
		return wrapKeyringError(err)
	}
	return nil
}

// deleteKeyringSecret 从密钥环删除凭据。
func deleteKeyringSecret(service, ref string) error {
	err := keyring.Delete(service, ref)
	if err != nil {
		// 删除不存在的条目不算错误
		if isKeyringNotFound(err) {
			return nil
		}
		return wrapKeyringError(err)
	}
	return nil
}

// isKeyringNotFound 检测是否为「条目不存在」错误。
func isKeyringNotFound(err error) bool {
	if err == nil {
		return false
	}
	// zalando/go-keyring 返回的标准错误
	return err.Error() == "secret not found in keyring" ||
		err.Error() == "keyring entry not found" ||
		err.Error() == keyring.ErrNotFound.Error()
}