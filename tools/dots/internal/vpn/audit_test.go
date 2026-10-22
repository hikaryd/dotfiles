package vpn

import (
	"strings"
	"testing"
)

func TestVlessTransportSecurity(t *testing.T) {
	for _, security := range []string{"none", "tls", "reality"} {
		t.Run(security, func(t *testing.T) {
			n := mustNode(t, strings.Replace(vlessURI, "security=reality", "security="+security, 1), 0)
			tls := obj(n["tls"])
			if security == "none" {
				if tls != nil && tls["enabled"] == true {
					t.Fatal("plaintext node changed to TLS")
				}
				return
			}
			if tls["enabled"] != true {
				t.Fatal("TLS missing")
			}
			reality := obj(tls["reality"])
			if security == "tls" && reality["enabled"] == true {
				t.Fatal("ordinary TLS changed to Reality")
			}
			if security == "reality" && reality["enabled"] != true {
				t.Fatal("Reality missing")
			}
		})
	}
	for _, query := range []string{"security=invalid", "security=reality&pbk=", "security=reality&sni="} {
		input := strings.Replace(vlessURI, "security=reality", query, 1)
		if strings.Contains(query, "pbk=") {
			input = strings.ReplaceAll(input, "&pbk=fake-public-key", "")
		}
		if strings.Contains(query, "sni=") {
			input = strings.ReplaceAll(input, "&sni=cover.example", "")
		}
		if _, err := parseVless(strings.TrimPrefix(input, "vless://"), 0); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
}
