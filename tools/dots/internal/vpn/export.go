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

// Выгружает свежие узлы для изолированных замеров, не трогая конфиг шлюза,
// без скачивания rule sets и перезапуска сервисов.
func exportNodes(ctx context.Context, args []string, out, errout io.Writer) int {
	flags := flag.NewFlagSet("nodes", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("output", "", "private output file")
	confPath := flags.String("env", env("VPN_ENV", "/etc/sing-box/vpn.env"), "subscription environment")
	usage := func() {
		fmt.Fprintln(out, "Использование: dots-vpn nodes --output FILE [--env FILE]\nСкачивает подписку в приватный конфиг для замеров; сервисы не трогает.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage()
			return 0
		}
		fmt.Fprintln(errout, "nodes: неверные аргументы, см. --help")
		return 2
	}
	if *path == "" || flags.NArg() != 0 {
		fmt.Fprintln(errout, "nodes: нужен --output FILE")
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
