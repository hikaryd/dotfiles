package fakecmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"golang.org/x/term"
)

type Rule struct {
	Args    []string
	Prefix  bool
	Output  string
	Outputs []string
	Stderr  string
	Code    int
	Block   bool
}

type Call struct {
	Name  string
	Args  []string
	Input string
	Env   map[string]string
}

func Run(dir string) (int, bool) {
	if dir == "" {
		return 0, false
	}
	name := filepath.Base(os.Args[0])
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return 0, false
	}
	var rules []Rule
	if json.Unmarshal(b, &rules) != nil {
		return 98, true
	}
	var input []byte
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		input, _ = io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	}
	call := Call{Name: name, Args: os.Args[1:], Input: string(input), Env: map[string]string{}}
	for _, key := range []string{"DOTS_ROOT", "DOTS_ZSH_BIN", "PATH", "GIT_OPTIONAL_LOCKS", "FZF_DEFAULT_OPTS", "FZF_DEFAULT_OPTS_FILE"} {
		call.Env[key] = os.Getenv(key)
	}
	f, err := os.OpenFile(filepath.Join(dir, "calls"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 98, true
	}
	err = json.NewEncoder(f).Encode(call)
	f.Close()
	if err != nil {
		return 98, true
	}
	for i, rule := range rules {
		match := reflect.DeepEqual(rule.Args, os.Args[1:])
		if rule.Prefix && len(os.Args)-1 >= len(rule.Args) {
			match = reflect.DeepEqual(rule.Args, os.Args[1:1+len(rule.Args)])
		}
		if !match {
			continue
		}
		out := rule.Output
		if len(rule.Outputs) > 0 {
			counter := filepath.Join(dir, fmt.Sprintf("%s.%d.counter", name, i))
			b, _ := os.ReadFile(counter)
			n, _ := strconv.Atoi(string(b))
			out = rule.Outputs[min(n, len(rule.Outputs)-1)]
			if os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0600) != nil {
				return 98, true
			}
		}
		if rule.Block {
			if os.WriteFile(filepath.Join(dir, "blocked-"+name), []byte("ready"), 0600) != nil {
				return 98, true
			}
			time.Sleep(time.Minute)
		}
		fmt.Fprint(os.Stdout, out)
		fmt.Fprint(os.Stderr, rule.Stderr)
		return rule.Code, true
	}
	fmt.Fprintln(os.Stderr, "unexpected fixture command")
	return 97, true
}

func Install(t testing.TB, dir, name string, rules ...Rule) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		if err := os.Symlink(exe, path); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

func Calls(t testing.TB, dir string) []Call {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []Call
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var call Call
		err := decoder.Decode(&call)
		if err == io.EOF {
			return calls
		}
		if err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
}
