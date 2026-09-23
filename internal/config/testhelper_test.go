package config

import "os"

// writeFile 供测试使用（避免在多个测试里重复 import os）。
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
