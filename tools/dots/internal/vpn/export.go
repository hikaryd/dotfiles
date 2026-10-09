package vpn

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Export fresh supported nodes for isolated measurements, without rewriting the
// gateway configuration, downloading rule sets, or restarting any services.
func exportNodes(ctx context.Context, args []string, out, errout io.Writer) int {
	flags := flag.NewFlagSet("nodes", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("output", "", "private output file")
	confPath := flags.String("env", env("VPN_ENV", "/etc/sing-box/vpn.env"), "subscription environment")
	usage := func() {
		fmt.Fprintln(out, "usage: dots-vpn nodes --output FILE [--env FILE]\nDownload a fresh subscription into a private benchmark config; no service changes.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage()
			return 0
		}
		fmt.Fprintln(errout, "nodes: invalid arguments; use --help")
		return 2
	}
	if *path == "" || flags.NArg() != 0 {
		fmt.Fprintln(errout, "nodes: --output FILE is required")
		return 2
	}
	fail := func() int {
		fmt.Fprintln(errout, "nodes: cannot export subscription; existing configuration preserved")
		return 1
	}
	if info, err := os.Lstat(*path); err == nil && !info.Mode().IsRegular() || err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail()
	}
	conf, err := loadConf(*confPath)
	if err != nil || conf["SUB_URL"] == "" {
		return fail()
	}
	raw, err := readURL(ctx, &http.Client{Timeout: 30 * time.Second}, conf["SUB_URL"], 5<<20, "sing-box")
	if err != nil {
		return fail()
	}
	nodes := parseSubscription(string(raw), errout)
	if len(nodes) == 0 {
		return fail()
	}
	if err := atomicWrite(*path, append(jsonBytes(object{"outbounds": nodes}), '\n'), 0600); err != nil {
		return fail()
	}
	lines := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	fmt.Fprintf(out, "Строк: %d; разобрано: %d; пропущено: %d\n", lines, len(nodes), lines-len(nodes))
	return 0
}
