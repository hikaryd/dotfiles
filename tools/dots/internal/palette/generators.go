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
	case "tmux":
		relative = "tmux/tmux.conf"
		text, err := readText(g.path(relative))
		if err != nil {
			return err
		}
		border, err := g.color("ui", "border")
		if err != nil {
			return err
		}
		accent, err := g.color("common", "accent")
		if err != nil {
			return err
		}
		text = replace(text, `pane-border-style 'fg=#[0-9a-fA-F]+'`, "pane-border-style 'fg="+border+"'")
		text = replace(text, `pane-active-border-style 'fg=#[0-9a-fA-F]+'`, "pane-active-border-style 'fg="+accent+"'")
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
	case "nushell", "fresh", "jcode":
		switch name {
		case "nushell":
			relative = "nushell/ayu.nu"
		case "fresh":
			relative = "fresh/themes/ayu-" + g.variant + ".json"
			mkdir = true
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
