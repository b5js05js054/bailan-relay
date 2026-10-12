//go:build windows

package license

import (
	"strings"

	"bailan-relay/internal/sysutil"
)

// machineGUID 读取注册表 MachineGuid 作为设备唯一标识，读不到返回空串。
func machineGUID() string {
	out, _, err := sysutil.RunHidden("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "MachineGuid") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					return fields[len(fields)-1]
				}
			}
		}
	}
	return ""
}
