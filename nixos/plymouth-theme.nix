{pkgs, ...}: {
  boot.plymouth = {
    enable = true;
    themePackages = [(pkgs.catppuccin-plymouth.override {
      variant = "mocha"; # Можно выбрать: latte, frappe, macchiato, mocha
      accent = "blue";   # Акцентный цвет
    })];
    theme = "catppuccin-mocha";
  };
}
