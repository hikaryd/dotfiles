// Firefox preferences managed by ~/dots

// Required for userChrome.css themes such as Firefox-Mod-Blur.
user_pref("toolkit.legacyUserProfileCustomizations.stylesheets", true);

// Avoid first-run/default-browser interruptions during automated setup.
user_pref("browser.aboutConfig.showWarning", false);
user_pref("browser.shell.checkDefaultBrowser", false);
user_pref("browser.shell.didSkipDefaultBrowserCheckOnFirstRun", true);
user_pref("browser.startup.homepage", "about:blank");
user_pref("browser.startup.page", 3);

// Prefer dark browser/content surfaces.
user_pref("browser.theme.content-theme", 0);
user_pref("browser.theme.toolbar-theme", 0);
user_pref("layout.css.prefers-color-scheme.content-override", 0);

// Firefox-Mod-Blur: keep macOS window controls on the left.
user_pref("mod.macos", true);

// Required by Firefox-Mod-Blur for SVG button icons.

// Required by Firefox-Mod-Blur for SVG button icons.
user_pref("svg.context-properties.content.enabled", true);

// Firefox-Mod-Blur: enable new-tab wallpaper blur, intensity 1..10.
user_pref("mod.new-tab.background.blur", 5);
