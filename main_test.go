package main

import "testing"

func TestUDPListenAddressUsesPortWhenAddressIsEmpty(t *testing.T) {
	if got := udpListenAddress("", 9000); got != ":9000" {
		t.Fatalf("udpListenAddress() = %q, want %q", got, ":9000")
	}
}

func TestUDPListenAddressTrimsAndPrefersAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{
			name:    "hostless",
			address: " :9100 ",
			want:    ":9100",
		},
		{
			name:    "ipv4 all interfaces",
			address: "0.0.0.0:9000",
			want:    "0.0.0.0:9000",
		},
		{
			name:    "ipv6 all interfaces",
			address: "[::]:9000",
			want:    "[::]:9000",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := udpListenAddress(test.address, 9000); got != test.want {
				t.Fatalf("udpListenAddress() = %q, want %q", got, test.want)
			}
		})
	}
}
