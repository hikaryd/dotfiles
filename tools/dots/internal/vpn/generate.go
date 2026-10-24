package vpn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ruleSets = map[string]string{"geoip-ru": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs", "geosite-ads": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs"}

func ensureSRS(ctx context.Context, etc, tag string, out, errout io.Writer) error {
	path := filepath.Join(etc, tag+".srs")
	s, e := os.Stat(path)
	if e == nil && s.Size() > 0 && time.Since(s.ModTime()) < 7*24*time.Hour {
		return nil
	}
	b, e := readURL(ctx, &http.Client{Timeout: 30 * time.Second}, ruleSets[tag], 32<<20, "sing-box")
	if e == nil {
		e = atomicWrite(path, b, 0600)
	}
	if e != nil {
		fmt.Fprintf(errout, "%s download failed: %s\n", tag, e)
		if s == nil || s.Size() == 0 {
			return e
		}
		return nil
	}
	fmt.Fprintf(out, "%s.srs updated (%d bytes)\n", tag, len(b))
	return nil
}
func buildConfig(nodes []object, auto []string, conf map[string]string, etc, secret string) (object, error) {
	ports := []int{}
	for i, k := range []string{"TPROXY_PORT", "CLASH_PORT", "HEALTH_PORT"} {
		p, e := strconv.Atoi(value(conf, k, []string{"7895", "9090", "7896"}[i]))
		if e != nil || p < 1 || p > 65535 {
			return nil, errors.New("VPN ports must be in 1..65535")
		}
		ports = append(ports, p)
	}
	if ports[0] == ports[1] || ports[0] == ports[2] || ports[1] == ports[2] {
		return nil, errors.New("TPROXY_PORT, CLASH_PORT and HEALTH_PORT must differ")
	}
	if secret == "" {
		return nil, errors.New("Panel secret must not be empty")
	}
	domains := []string{"aquadx.hydev.org"}
	dnsRules := []object{{"domain": domains, "server": "proxy-dns"}}
	sets := []object{{"type": "local", "tag": "geoip-ru", "format": "binary", "path": filepath.Join(etc, "geoip-ru.srs")}}
	if value(conf, "ADBLOCK", "1") == "1" {
		whitelist := []string{}
		for _, d := range strings.Split(value(conf, "ADBLOCK_WHITELIST", "tmdb.org,themoviedb.org,b-cdn.net"), ",") {
			if d = strings.TrimSpace(d); d != "" {
				whitelist = append(whitelist, d)
			}
		}
		if len(whitelist) > 0 {
			dnsRules = append(dnsRules, object{"domain_suffix": whitelist, "server": "proxy-dns"})
		}
		dnsRules = append(dnsRules, object{"rule_set": "geosite-ads", "action": "reject"})
		sets = append(sets, object{"type": "local", "tag": "geosite-ads", "format": "binary", "path": filepath.Join(etc, "geosite-ads.srs")})
	}
	dnsRules = append(dnsRules, object{"clash_mode": "direct", "server": "direct-dns"}, object{"clash_mode": "global", "server": "proxy-dns"})
	tags := []string{"auto"}
	for _, n := range nodes {
		tags = append(tags, str(n["tag"]))
	}
	outbounds := []object{{"type": "selector", "tag": "select", "outbounds": tags, "default": "auto"}, {"type": "urltest", "tag": "auto", "outbounds": auto, "url": "https://www.gstatic.com/generate_204", "interval": "5m", "tolerance": 50}}
	outbounds = append(outbounds, nodes...)
	outbounds = append(outbounds, object{"type": "direct", "tag": "direct"})
	return object{
		"log": object{"level": "warn", "timestamp": true}, "dns": object{"servers": []object{{"tag": "proxy-dns", "type": "https", "server": "1.1.1.1", "detour": "select"}, {"tag": "direct-dns", "type": "local", "detour": "direct"}}, "rules": dnsRules, "final": "proxy-dns", "strategy": "prefer_ipv4"},
		"inbounds": []object{{"type": "tproxy", "tag": "tproxy-in", "listen": "0.0.0.0", "listen_port": ports[0]}, {"type": "mixed", "tag": "health-in", "listen": "127.0.0.1", "listen_port": ports[2]}}, "outbounds": outbounds,
		"route":        object{"rules": []object{{"action": "sniff"}, {"protocol": "dns", "action": "hijack-dns"}, {"inbound": []string{"health-in"}, "action": "route", "outbound": "select"}, {"domain": domains, "action": "route", "outbound": "select"}, {"ip_is_private": true, "action": "route", "outbound": "direct"}, {"rule_set": "geoip-ru", "action": "route", "outbound": "direct"}}, "rule_set": sets, "final": "select", "auto_detect_interface": true, "default_domain_resolver": object{"server": "direct-dns"}},
		"experimental": object{"clash_api": object{"external_controller": fmt.Sprintf("0.0.0.0:%d", ports[1]), "secret": secret, "external_ui": filepath.Join(etc, "ui"), "default_mode": "rule"}, "cache_file": object{"enabled": true, "path": "/var/lib/sing-box/cache.db"}}}, nil
}
func generate(ctx context.Context, confPath, etc string, out, errout io.Writer) error {
	conf, e := loadConf(confPath)
	if e != nil {
		return e
	}
	if conf["SUB_URL"] == "" {
		return errors.New("SUB_URL is empty in " + confPath)
	}
	for _, tag := range []string{"geoip-ru", "geosite-ads"} {
		if tag == "geosite-ads" && value(conf, "ADBLOCK", "1") != "1" {
			continue
		}
		if e = ensureSRS(ctx, etc, tag, out, errout); e != nil {
			return e
		}
	}
	source, e := readURL(ctx, &http.Client{Timeout: 30 * time.Second}, conf["SUB_URL"], 5<<20, "sing-box")
	if e != nil {
		return e
	}
	nodes := parseSubscription(string(source), errout)
	if len(nodes) == 0 {
		return errors.New("NO NODES PARSED — keeping old config")
	}
	limit, e := strconv.Atoi(value(conf, "AUTO_MAX", "8"))
	if e != nil {
		return errors.New("AUTO_MAX must be positive")
	}
	auto, e := autoPool(nodes, conf["AUTO_INCLUDE"], value(conf, "AUTO_EXCLUDE", "LTE|4G|3G|Обход БС|⬇|⬆"), limit, conf["AUTO_PREFER"])
	if e != nil {
		return e
	}
	secret, e := os.ReadFile(filepath.Join(etc, "secret.txt"))
	if e != nil {
		return errors.New("cannot read panel secret")
	}
	config, e := buildConfig(nodes, auto, conf, etc, strings.TrimSpace(string(secret)))
	if e != nil {
		return e
	}
	if e = validateAndReplace(ctx, etc, config); e != nil {
		return e
	}
	ad := "off"
	if value(conf, "ADBLOCK", "1") == "1" {
		ad = "on"
	}
	fmt.Fprintf(out, "OK: %d manual nodes, %d auto nodes, adblock=%s\n", len(nodes), len(auto), ad)
	return nil
}

func validateAndReplace(ctx context.Context, etc string, config object) error {
	tmp, e := os.CreateTemp(etc, ".config-*.json")
	if e != nil {
		return errors.New("cannot create private config")
	}
	defer os.Remove(tmp.Name())
	_, e = tmp.Write(jsonBytes(config))
	ce := tmp.Close()
	if e != nil || ce != nil {
		return errors.New("cannot write private config")
	}
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, code, e := command(c, "", "sing-box", "check", "-c", tmp.Name())
	if e != nil || code != 0 {
		return fmt.Errorf("CONFIG CHECK FAILED (exit %d) — keeping old config", code)
	}
	if e = os.Rename(tmp.Name(), filepath.Join(etc, "config.json")); e != nil {
		return errors.New("cannot replace config")
	}
	return nil
}
