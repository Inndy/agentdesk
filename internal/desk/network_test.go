package desk

import (
	"errors"
	"testing"
)

func TestResolveNetwork(t *testing.T) {
	for _, tc := range []struct {
		name, listen, public      string
		explicit, telegram        bool
		wantListen, wantAdvertise string
		wantError, detect         bool
	}{
		{"local default", "127.0.0.1:7331", "", false, false, "127.0.0.1:7331", "", false, false},
		{"Telegram auto NIC", "127.0.0.1:7331", "", false, true, "192.168.10.8:7331", "192.168.10.8", false, true},
		{"explicit VPN", "100.64.1.2:9000", "", true, true, "100.64.1.2:9000", "", false, false},
		{"explicit loopback needs URL", "127.0.0.1:9000", "", true, true, "", "", true, false},
		{"proxy override", "127.0.0.1:9000", "https://issues.example.com", true, true, "127.0.0.1:9000", "", false, false},
		{"wildcard auto URL", "0.0.0.0:9000", "", true, true, "0.0.0.0:9000", "192.168.10.8", false, true},
		{"invalid URL", "127.0.0.1:7331", "javascript:bad", false, true, "", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			listen, advertise, err := ResolveNetwork(tc.listen, tc.public, tc.explicit, tc.telegram, func() (NetworkAddress, error) {
				called = true
				return NetworkAddress{Interface: "ethernet", Address: "192.168.10.8"}, nil
			})
			if (err != nil) != tc.wantError || listen != tc.wantListen || advertise != tc.wantAdvertise || called != tc.detect {
				t.Fatalf("unexpected network resolution: %s %s %v (detect=%v)", listen, advertise, err, called)
			}
		})
	}
	if _, _, err := ResolveNetwork("127.0.0.1:7331", "", false, true, func() (NetworkAddress, error) { return NetworkAddress{}, errors.New("offline") }); err == nil {
		t.Fatal("failed detection silently advertised localhost")
	}
	if _, _, err := ResolveNetwork("0.0.0.0:7331", "", true, true, func() (NetworkAddress, error) { return NetworkAddress{Address: "2001:db8::1"}, nil }); err == nil {
		t.Fatal("IPv4-only listener advertised IPv6 address")
	}
}
