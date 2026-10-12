//go:build darwin

package license

import (
	"strings"

	"bailan-relay/internal/sysutil"
)

// machineGUID 读取硬件 IOPlatformUUID 作为设备唯一标识，读不到返回空串。
func machineGUID() string {
	out, _, err := sysutil.RunHidden("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "IOPlatformUUID") {
			continue
		}
		if _, v, ok := strings.Cut(line, "="); ok {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}
