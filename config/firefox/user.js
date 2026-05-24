// Firefox preferences managed by ~/dots

// Required for userChrome.css themes such as CascadeFox/Cascade.
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
