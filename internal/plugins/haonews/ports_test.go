package newsplugin

import "testing"

func TestPickFreeTCPPortAvoidsPrivilegedRange(t *testing.T) {
	port, err := pickFreeTCPPort()
	if err != nil {
		t.Fatalf("pickFreeTCPPort() error = %v", err)
	}
	if port < minimumDynamicPort {
		t.Fatalf("pickFreeTCPPort() = %d, want >= %d", port, minimumDynamicPort)
	}
}
