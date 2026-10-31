package main

import (
	"testing"

	"agentdesk/internal/desk"
)

func TestServerOptionsReuseProjectPort(t *testing.T) {
	c := desk.Config{Endpoint: "http://192.168.29.24:7332", Server: desk.ServerDefaults{Telegram: true}}
	for _, command := range []string{"start", "serve"} {
		t.Run(command, func(t *testing.T) {
			_, o, _, err := parse([]string{command})
			if err != nil {
				t.Fatal(err)
			}
			o = serverOptions(c, o)
			listen, _, err := desk.ResolveNetwork(o.listen, o.publicURL, o.listenExplicit, o.telegram, func() (desk.NetworkAddress, error) {
				return desk.NetworkAddress{Address: "192.168.29.25"}, nil
			})
			if err != nil || listen != "192.168.29.25:7332" {
				t.Fatalf("project port or NIC autodetection lost: %q, %v", listen, err)
			}
		})
	}
	for _, tc := range []struct {
		name, saved, explicit, want string
	}{
		{"saved listener", "127.0.0.1:7444", "", "127.0.0.1:7444"},
		{"explicit listener", "127.0.0.1:7444", "127.0.0.1:7555", "127.0.0.1:7555"},
		{"ephemeral listener", "", "127.0.0.1:0", "127.0.0.1:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.Server.Listen = tc.saved
			args := []string{"start"}
			if tc.explicit != "" {
				args = append(args, "--listen", tc.explicit)
			}
			_, o, _, _ := parse(args)
			if got := serverOptions(c, o).listen; got != tc.want {
				t.Fatalf("listen = %q, want %q", got, tc.want)
			}
		})
	}
}
