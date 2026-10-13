//go:build windows

package devin

import (
	"strings"
	"testing"
)

// Devin main.js 会用 *.localhost 覆盖 --host-resolver-rules，只有 --host-rules 能原样传到 NetworkService。
func TestWindowsArgsCarriesQuotedHostRulesAndResolverRules(t *testing.T) {
	args := windowsArgs()
	rules := hostResolverRules()
	if rules == "" {
		t.Fatal("hostResolverRules should not be empty with default hijack domains")
	}
	for _, flag := range []string{"--host-rules", "--host-resolver-rules"} {
		if want := flag + `="` + rules + `"`; !strings.Contains(args, want) {
			t.Fatalf("windowsArgs missing %s\nargs: %s", want, args)
		}
	}
	if !strings.Contains(args, "--disable-quic") {
		t.Fatalf("windowsArgs lost --disable-quic: %s", args)
	}
}
