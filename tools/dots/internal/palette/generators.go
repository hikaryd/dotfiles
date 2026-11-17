package palette

import (
	"fmt"
)

func (g generator) generateTool(name string) error {
	ctx := g.context()
	relative := ""
	mkdir := false
	switch name {
	case "starship":
		relative = "starship/starship.toml"
		text, err := readText(g.path(relative))
		if err != nil {
			return err
		}
		content, err := render(name, ctx)
		if err != nil {
			return err
		}
		text = replace(text, `(?m)^palette\s*=\s*".*"`, `palette = "ayu_`+g.variant+`"`)
		text = replaceStarshipPalette(text, content)
		if err := g.write(relative, text, false); err != nil {
			return err
		}
	case "tmux", "gitk", "fastfetch":
		var err error
		text := ""
		switch name {
		case "tmux":
			relative = "tmux/tmux.conf"
		case "gitk":
			relative = "git/gitk"
		case "fastfetch":
			relative = "fastfetch/config.jsonc"
		}
		if text, err = readText(g.path(relative)); err != nil {
			return err
		}
		switch name {
		case "tmux":
			text, err = replaceLines(text, "set -g ", "'", tmuxLines, ctx)
		case "gitk":
			text, err = replaceLines(text, "set ", "", gitkLines, ctx)
		case "fastfetch":
			text, err = replaceFastfetch(text, ctx)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := g.write(relative, text, false); err != nil {
			return err
		}
	case "lazygit":
		relative = "lazygit/config.yml"
		text, err := readText(g.path(relative))
		if err != nil {
			return err
		}
		themeBlock, err := render(name, ctx)
		if err != nil {
			return err
		}
		text, err = replaceLazygitTheme(text, themeBlock)
		if err != nil {
			return err
		}
		if err := g.write(relative, text, false); err != nil {
			return err
		}
	case "yazi":
		return g.yazi()
	case "nushell", "jcode", "nvim", "zsh":
		switch name {
		case "nushell":
			relative = "nushell/ayu.nu"
		case "nvim":
			relative = "nvim/lua/core/palette.lua"
		case "zsh":
			relative = "zsh/palette.zsh"
		case "jcode":
			relative = "jcode/theme.toml"
			mkdir = true
		}
		content, err := render(name, ctx)
		if err != nil {
			return err
		}
		if err := g.write(relative, content, mkdir); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown generator %s", name)
	}
	fmt.Fprintf(g.out, "  %s: %s\n", name, g.path(relative))
	return nil
}
