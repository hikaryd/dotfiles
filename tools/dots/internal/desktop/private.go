package desktop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf8"
)

const gitTemplate = `# Личная Git-идентичность; раскомментируйте и заполните.
# [user]
#     name = ВАШЕ_ИМЯ
#     email = ВАША_ПОЧТА
# Репозитории обслуживания можно добавить секцией [maintenance], ключ repo.
`

type privateLocation struct{ label, path, template string }

func privateLocations() []privateLocation {
	return []privateLocation{{"shell", envOr("DOTS_PRIVATE_ZSH", filepath.Join(configHome(), "dots/private.zsh")), "config/zsh/private.example.zsh"}, {"git", filepath.Join(home(), ".config/git/private.inc"), ""}}
}
func validatePrivatePath(root, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("нужен абсолютный путь")
	}
	info, e := os.Lstat(path)
	if e == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("символическая ссылка: проверьте вручную")
	}
	if e != nil && !os.IsNotExist(e) {
		return errors.New("недоступный путь")
	}
	if within(resolve(path), resolve(root)) {
		return errors.New("приватный файл не может находиться в репозитории")
	}
	if e == nil && !info.Mode().IsRegular() {
		return errors.New("ожидался обычный файл")
	}
	return nil
}
func openNoFollow(path string, flags int, mode uint32) (*os.File, error) {
	fd, e := syscall.Open(path, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, mode)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), path), nil
}
func checkPrivate(path string) error {
	f, e := openNoFollow(path, syscall.O_RDONLY, 0)
	if e != nil {
		return errors.New("файл отсутствует, недоступен или не является текстом")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return errors.New("файл недоступен")
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(s.Uid) != os.Getuid() {
		return errors.New("нужен обычный файл текущего пользователя")
	}
	if info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("установите права 600; автоматически не меняю")
	}
	b, e := io.ReadAll(f)
	if e != nil || !utf8.Valid(b) {
		return errors.New("файл отсутствует, недоступен или не является текстом")
	}
	lines := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			lines = append(lines, line)
		}
	}
	active := strings.Join(lines, "\n")
	if active == "" || regexp.MustCompile(`your-|example-|\b(?:dev|stage|preprod|prod)-context\b|ВАШ[АЕ]`).MatchString(active) {
		return errors.New("шаблон не заполнен или остались примеры")
	}
	if !regexp.MustCompile(`=[ \t]*[^ \t'"\n]|=[ \t]*['"][^'"\n]`).MatchString(active) {
		return errors.New("нет заполненных значений")
	}
	return nil
}
func privateRun(root string, args []string) int {
	action := "check"
	if len(args) == 1 {
		action = args[0]
	} else if len(args) > 1 {
		return usageError("private")
	}
	if !has(action, "init", "check") {
		return usageError("private")
	}
	entries := privateLocations()
	failed := false
	for _, entry := range entries {
		if e := validatePrivatePath(root, entry.path); e != nil {
			fmt.Printf("%s: %s\n", entry.label, e)
			failed = true
		}
	}
	if failed {
		return 1
	}
	for _, entry := range entries {
		display := strings.Replace(entry.path, home(), "~", 1)
		var e error
		if action == "init" {
			e = os.MkdirAll(filepath.Dir(entry.path), 0700)
			if e == nil {
				var f *os.File
				f, e = openNoFollow(entry.path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL, 0600)
				if os.IsExist(e) {
					fmt.Printf("%s: сохранён существующий файл (%s)\n", entry.label, display)
					continue
				}
				if e == nil {
					b := []byte(gitTemplate)
					if entry.template != "" {
						b, e = os.ReadFile(filepath.Join(root, entry.template))
					}
					if e == nil {
						_, e = f.Write(b)
					}
					ce := f.Close()
					if e == nil {
						e = ce
					}
					if e != nil {
						os.Remove(entry.path)
					}
				}
			}
			if e == nil {
				fmt.Printf("%s: создан шаблон, заполните (%s)\n", entry.label, display)
			} else {
				e = errors.New("файл отсутствует, недоступен или не является текстом")
			}
		} else {
			e = checkPrivate(entry.path)
			if e == nil {
				fmt.Printf("%s: права и базовая проверка заполнения в порядке (%s)\n", entry.label, display)
			}
		}
		if e != nil {
			fmt.Printf("%s: %s (%s)\n", entry.label, e, display)
			failed = true
		}
	}
	if action == "check" {
		fmt.Println("Значения не выводятся и не исполняются. Доступность сервисов и действительность ключей не проверяются.")
	}
	if failed {
		return 1
	}
	return 0
}
