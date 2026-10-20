"""Regressions for the reported blank overview: no scope, scope changes and layout.
All HTTP(S) is intercepted. No credentials, real sites or main browser profile.
"""
import json
import os
import re
import tempfile
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
BINARY = os.environ.get("BRAVE_BINARY", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser")
with tempfile.TemporaryDirectory(prefix="overview-regression-") as profile, sync_playwright() as p:
    context = p.chromium.launch_persistent_context(profile, executable_path=BINARY, headless=True,
        viewport={"width": 1440, "height": 900},
        args=[f"--disable-extensions-except={ROOT}", f"--load-extension={ROOT}"])
    try:
        context.route(re.compile(r"^https?://"), lambda route: route.fulfill(status=200, content_type="text/html", body="<title>Synthetic task</title>"))
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker")
        extension_id = worker.url.split("/")[2]
        first = worker.evaluate("async () => (await chrome.windows.getAll({windowTypes:['normal']}))[0].id")
        with context.expect_page() as new_window:
            second = worker.evaluate("async () => (await chrome.windows.create({url:'about:blank',focused:false})).id")
        new_window.value.wait_for_load_state()
        scope = {"first": first, "second": second}
        for window_id, card_id in [(scope['first'], 101), (scope['second'], 202)]:
            with context.expect_page() as opened:
                tab_id = worker.evaluate("async windowId => (await chrome.tabs.create({windowId,url:'about:blank'})).id", window_id)
            opened.value.goto(f"https://kaiten.x5.ru/space/1/card/{card_id}")
            worker.evaluate("""async ({windowId,tabId,id}) => {
                const groupId=await chrome.tabs.group({tabIds:[tabId],createProperties:{windowId}});
                await chrome.tabGroups.update(groupId,{title:`Задача ${id} `+'ОченьДлинноеНазвание'.repeat(12)+' 🔒',color:'purple',collapsed:true});
            }""", {"windowId": window_id, "tabId": tab_id, "id": card_id})
        page = context.new_page()
        errors = []
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.goto(f"chrome-extension://{extension_id}/overview.html")
        page.wait_for_selector("#window option", state="attached")
        # The exact screenshot failure: populated-looking picker, but no actual selected scope.
        expect(page.locator("#window")).to_have_value("")
        expect(page.locator("#onboarding")).to_be_visible()
        expect(page.locator("#filters")).to_be_hidden()
        expect(page.locator(".column-head")).to_be_hidden()
        expect(page.locator("#status")).not_to_contain_text("0 из 0")
        expect(page.locator("#window-choices button")).to_have_count(2)
        # Selecting an option is sufficient. No second Apply/Select button or hidden draft state.
        page.select_option("#window", str(scope["first"]))
        expect(page.locator(".task")).to_have_count(1)
        expect(page.locator("#onboarding")).to_be_hidden()
        expect(page.locator("#filters")).to_be_visible()
        expect(page.locator("#window")).to_have_value(str(scope["first"]))
        expect(page.locator(".task")).to_contain_text("#101")
        # A zero-result filter must not make the next selected window appear empty.
        page.click('[data-filter="done"]')
        expect(page.locator(".task")).to_have_count(0)
        expect(page.locator("#clear-filters")).to_be_visible()
        page.select_option("#window", str(scope["second"]))
        expect(page.locator(".task")).to_have_count(1)
        expect(page.locator(".task")).to_contain_text("#202")
        expect(page.locator('[data-filter="all"]')).to_have_attribute("aria-pressed", "true")
        # Scope changes made from the popup/background also reach an already-open dashboard.
        page.evaluate("id => chrome.runtime.sendMessage({type:'select',windowId:id})", scope["first"])
        expect(page.locator(".task")).to_contain_text("#101")
        expect(page.locator("#window")).to_have_value(str(scope["first"]))
        page.select_option("#window", str(scope["second"]))
        expect(page.locator(".task")).to_contain_text("#202")
        # A background rescan cannot revert the dropdown to an old scope.
        page.click("#refresh")
        expect(page.locator("#refresh")).to_be_enabled()
        expect(page.locator("#window")).to_have_value(str(scope["second"]))
        for width in [320, 480, 768, 1024, 1440, 2560]:
            page.set_viewport_size({"width": width, "height": 900})
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), f"Horizontal overflow at {width}"
            assert page.locator(".task").first.bounding_box()["y"] < (650 if width < 768 else 450), f"Settings push tasks below fold at {width}"
            if width in [480, 1440, 2560]:
                page.screenshot(path=f"/tmp/overview-validation-{width}.png", full_page=True)
        # Browser restart clears session scope. CLI-loaded extensions cannot runtime.reload()
        # (Chromium unloads them); restart the same profile to exercise the real lifecycle.
        context.close()
        context = p.chromium.launch_persistent_context(profile, executable_path=BINARY, headless=True,
            viewport={"width": 1440, "height": 900},
            args=[f"--disable-extensions-except={ROOT}", f"--load-extension={ROOT}"])
        context.route(re.compile(r"^https?://"), lambda route: route.fulfill(status=200, content_type="text/html", body="<title>Synthetic task</title>"))
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker")
        worker.evaluate("""async () => {
            const tab=await chrome.tabs.create({url:'about:blank',active:false});
            const group=await chrome.tabs.group({tabIds:[tab.id]});
            await chrome.tabGroups.update(group,{title:'После перезапуска',color:'blue'});
        }""")
        page = context.new_page()
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.goto(f"chrome-extension://{extension_id}/overview.html")
        expect(page.locator("#onboarding")).to_be_visible()
        expect(page.locator("#window")).to_have_value("")
        expect(page.locator("#filters")).to_be_hidden()
        page.locator("#window-choices button").first.click()
        expect(page.locator(".task")).to_have_count(1)
        assert not errors, errors
        print(json.dumps({"ok": True, "checks": ["honest unselected state", "single-step window selection", "filter reset", "no picker rollback", "6 viewport widths", "browser restart onboarding"]}))
    finally:
        context.close()
