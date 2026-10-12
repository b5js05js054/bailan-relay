package devin

import (
	"reflect"
	"testing"
	"time"
)

func TestTerminateProcessTreeKillsEveryConfiguredImage(t *testing.T) {
	running := true
	var killed []string
	ok := terminateProcessTree(
		[]string{"Devin.exe", "language_server.exe"},
		func() bool { return running },
		func(name string) {
			killed = append(killed, name)
			if name == "language_server.exe" {
				running = false
			}
		},
		func(time.Duration) {},
		10*time.Second,
	)
	if !ok {
		t.Fatal("terminateProcessTree should report success after all images exit")
	}
	if !reflect.DeepEqual(killed, []string{"Devin.exe", "language_server.exe"}) {
		t.Fatalf("killed images = %#v", killed)
	}
}
