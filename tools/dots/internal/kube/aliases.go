package kube

import (
	"debug/buildinfo"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func aliasesCommand(args []string) error {
	flags := flag.NewFlagSet("install-aliases", flag.ContinueOnError)
	directory := flags.String("bin-dir", "", "directory for command links")
	yes := flags.Bool("yes", false, "approve installation of local command links")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return exitError(2)
	}
	if flags.NArg() != 0 {
		return errors.New("Usage: dots-kube install-aliases [--bin-dir DIR] --yes")
	}
	if !*yes {
		return errors.New("Alias installation requires explicit --yes (setup installs them after confirmation)")
	}
	return installAliases(*directory)
}

func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
func aliasDirectory(directory string) (string, error) {
	if directory != "" {
		return filepath.Abs(directory)
	}
	exe, err := executablePath()
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(exe)
	for _, path := range filepath.SplitList(os.Getenv("PATH")) {
		absolute, err := filepath.Abs(path)
		if err == nil && absolute == parent {
			return parent, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}
func ownedBinary(path, name string) bool {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return false
	}
	if info.Path == "dots/native/cmd/dots-kube" || info.Path == "dots/native/cmd/dots-tools" {
		return true
	}
	legacy := map[string]string{"kube-deploy-watch": "dots/kube-rollout", "kube-log-watch": "dots/kube-log-watch", "kube-pod-analyze": "dots/kube-pod-analyze"}
	return legacy[name] != "" && info.Path == legacy[name]
}
func checkAliasDirectory(directory string) error {
	dir, err := aliasDirectory(directory)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	canonical := filepath.Join(dir, "dots-kube")
	names := append([]string{"dots-kube"}, CommandNames()...)
	for _, name := range names {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && resolved == exe {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(link) {
				link = filepath.Join(dir, link)
			}
			if name != "dots-kube" && filepath.Clean(link) == canonical {
				continue
			}
		}
		if info.Mode().IsRegular() && ownedBinary(path, name) {
			continue
		}
		return fmt.Errorf("Existing command preserved: %s. Choose another --bin-dir", path)
	}
	return nil
}
func installAliases(directory string) error {
	if err := checkAliasDirectory(directory); err != nil {
		return err
	}
	dir, err := aliasDirectory(directory)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	canonical := filepath.Join(dir, "dots-kube")
	resolved, _ := filepath.EvalSymlinks(canonical)
	if resolved != exe {
		source, err := os.Open(exe)
		if err != nil {
			return err
		}
		defer source.Close()
		file, err := os.CreateTemp(dir, ".dots-kube-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		_, copyErr := io.Copy(file, source)
		modeErr := file.Chmod(0755)
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(copyErr, modeErr, syncErr, closeErr); err != nil {
			return err
		}
		if err := os.Rename(file.Name(), canonical); err != nil {
			return err
		}
	}
	for _, name := range CommandNames() {
		path := filepath.Join(dir, name)
		resolved, _ := filepath.EvalSymlinks(path)
		if resolved == exe || resolved == canonical {
			continue
		}
		// Recheck before replacing: a concurrent installation must not overwrite a
		// command that appeared after the initial preflight.
		if _, err := os.Lstat(path); err == nil {
			if !ownedBinary(path, name) {
				link, linkErr := os.Readlink(path)
				if linkErr != nil || filepath.Clean(filepath.Join(dir, link)) != canonical {
					return fmt.Errorf("Command changed during installation: %s", path)
				}
			}
			if err := os.Remove(path); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink("dots-kube", path); err != nil {
			return err
		}
	}
	fmt.Println("✓ Command links installed:", dir)
	inPath := false
	for _, path := range filepath.SplitList(os.Getenv("PATH")) {
		absolute, _ := filepath.Abs(path)
		if absolute == dir {
			inPath = true
		}
	}
	if !inPath {
		fmt.Printf("Add the directory to PATH (or use %s directly).\n", canonical)
		fmt.Printf("  export PATH='%s':\"$PATH\"\n", strings.ReplaceAll(dir, "'", "'\"'\"'"))
	}
	return nil
}
