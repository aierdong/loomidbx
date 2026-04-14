//go:build !windows && !darwin && !linux

package app

// isKeyringAvailable 在非支持平台直接返回不可用。
func isKeyringAvailable(service string) error {
	return ErrKeyringUnavailable
}

// getKeyringSecret 在非支持平台直接返回不可用。
func getKeyringSecret(service, ref string) (string, error) {
	return "", ErrKeyringUnavailable
}

// setKeyringSecret 在非支持平台直接返回不可用。
func setKeyringSecret(service, ref, secret string) error {
	return ErrKeyringUnavailable
}

// deleteKeyringSecret 在非支持平台直接返回不可用。
func deleteKeyringSecret(service, ref string) error {
	return ErrKeyringUnavailable
}

// isKeyringNotFound 在非支持平台永远返回 false。
func isKeyringNotFound(err error) bool {
	return false
}