package cliapp

import "testing"

func TestMachineCredentialsPresent(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"none", nil, false},
		{"token", map[string]string{"DDC_TOKEN": "t"}, true},
		{"client credentials", map[string]string{"DDC_SERVICE_CLIENT_ID": "id", "DDC_SERVICE_CLIENT_SECRET": "s"}, true},
		{"id without secret", map[string]string{"DDC_SERVICE_CLIENT_ID": "id"}, false},
		{"unread legacy variable", map[string]string{"DDC_CLIENT_ID_SECRET": "id:s"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"DDC_TOKEN", "DDC_SERVICE_CLIENT_ID", "DDC_SERVICE_CLIENT_SECRET", "DDC_CLIENT_ID_SECRET"} {
				t.Setenv(k, tc.env[k])
			}
			if got := MachineCredentialsPresent(); got != tc.want {
				t.Fatalf("MachineCredentialsPresent() = %v, want %v", got, tc.want)
			}
		})
	}
}
