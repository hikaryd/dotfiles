"""Real Brave/MV3 overview smoke. Temporary profiles; ALL HTTP(S) traffic is fixtures.
The second profile pre-grants exactly the optional site/scripting permissions via a
copied manifest, avoiding native permission prompts in headless automation.
"""
import json
import os
import re
import shutil
import tempfile
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
BINARY = os.environ.get("BRAVE_BINARY", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser")


def run(p, directory, enabled):
    extension = directory / "extension"
    shutil.copytree(ROOT, extension)
    if enabled:
        manifest = json.loads((extension / "manifest.json").read_text())
        manifest["permissions"] += manifest.pop("optional_permissions")
        manifest["host_permissions"] = manifest.pop("optional_host_permissions")
        (extension / "manifest.json").write_text(json.dumps(manifest))
    context = p.chromium.launch_persistent_context(
        str(directory / "profile"), executable_path=BINARY, headless=True,
        viewport={"width": 1440, "height": 1050},
        args=[f"--disable-extensions-except={extension}", f"--load-extension={extension}"],
    )
    errors = []
    api_requests = []
    fail_auth = False
    titles = {
        "https://kaiten.x5.ru/space/1/card/99": "Ускорить проверки | Kaiten",
        "https://scm.x5.ru/team/app/-/merge_requests/42": "ATS-1234 fix: Ускорить проверки",
        "https://kaiten.x5.ru/space/1/card/100": "Обновить контракт | Kaiten",
        "https://scm.x5.ru/team/docs/-/merge_requests/42": "ATS-5678 docs: Обновить контракт",
        "https://scm.x5.ru/team/service/-/merge_requests/18": "ATS-9012 feat: Новая интеграция",
    }

    def respond(route):
        url = route.request.url
        if "/api/" in url:
            api_requests.append({"url": url, "method": route.request.method})
            if fail_auth:
                route.fulfill(status=401, content_type="application/json", body="{}")
                return
            if "/cards/" in url:
                card_id = int(url.rsplit("/", 1)[1])
                done = card_id == 100
                body = {"id": card_id, "title": "Обновить контракт" if done else "Ускорить проверки", "state": 3 if done else 2,
                        "column": {"title": "Готово" if done else "В разработке"}, "board": {"title": "Команда / Разработка"},
                        "blocked": False, "members": [{"type": 2, "full_name": "Тестовый исполнитель"}]}
            else:
                done = "docs" in url
                draft = "service" in url
                body = {"iid": 18 if draft else 42, "state": "merged" if done else "opened", "draft": draft,
                        "title": "Обновить контракт" if done else "Новая интеграция" if draft else "Ускорить проверки",
                        "has_conflicts": False, "blocking_discussions_resolved": done or draft,
                        "head_pipeline": {"status": "success" if done else "manual" if draft else "failed"},
                        "reviewers": [{"name": "Тестовый ревьюер"}]}
            route.fulfill(status=200, content_type="application/json", body=json.dumps(body))
        else:
            route.fulfill(status=200, content_type="text/html; charset=utf-8", body=f"<meta charset='utf-8'><title>{titles.get(url, 'Fixture')}</title><p>Synthetic fixture</p>")

    context.route(re.compile(r"^https?://"), respond)
    try:
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker")
        extension_id = worker.url.split("/")[2]
        window_id = worker.evaluate("async () => (await chrome.windows.getAll({windowTypes:['normal']}))[0].id")
        tab_ids = []
        for url in titles:
            with context.expect_page() as created:
                tab_id = worker.evaluate("async windowId => (await chrome.tabs.create({windowId,url:'about:blank'})).id", window_id)
            created.value.goto(url)
            tab_ids.append(tab_id)
        group_ids = worker.evaluate("""async ({windowId,tabIds}) => {
            const ids=[];
            for (const [members,color] of [[tabIds.slice(0,2),'purple'],[tabIds.slice(2,4),'green'],[tabIds.slice(4),'blue']]) {
                const id=await chrome.tabs.group({tabIds:members});
                await chrome.tabGroups.update(id,{color,collapsed:true}); ids.push(id);
            }
            return ids;
        }""", {"windowId": window_id, "tabIds": tab_ids})
        popup = context.new_page()
        popup.goto(f"chrome-extension://{extension_id}/popup.html")
        expect(popup.locator("#overview")).to_be_enabled()
        with context.expect_page() as opened:
            popup.click("#overview")
        page = opened.value
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.wait_for_selector("#window option", state="attached")
        before_reopen = worker.evaluate("async () => (await chrome.tabs.query({})).length")
        page.evaluate("() => chrome.runtime.sendMessage({type:'overview-open'})")
        assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == before_reopen
        expect(page.locator("#empty")).to_contain_text("Выберите окно")
        page.select_option("#window", str(window_id))
        page.click("#select")
        expect(page.locator(".task")).to_have_count(3)
        expect(page.locator("#refresh")).to_be_enabled()
        if not enabled:
            expect(page.locator("#enable")).to_be_visible()
            expect(page.locator(".task").first).to_contain_text("Чтение статусов отключено")
            assert not api_requests
        else:
            expect(page.locator("#disable")).to_be_visible()
            expect(page.locator(".task").first).to_contain_text("CI упал")
            expect(page.locator(".task").nth(1)).to_contain_text("Merged")
            expect(page.locator(".task").nth(2)).to_contain_text("Draft")
            assert len(api_requests) == 5, api_requests
            assert all(request["method"] == "GET" for request in api_requests)
            page.click('[data-filter="done"]')
            expect(page.locator(".task")).to_have_count(1)
            expect(page.locator(".task")).to_contain_text("ATS-5678")
            page.click('[data-filter="attention"]')
            expect(page.locator(".task")).to_have_count(1)
            expect(page.locator(".task")).to_contain_text("ATS-1234")
            page.click('[data-filter="all"]')
            page.keyboard.press("/")
            expect(page.locator("#search")).to_be_focused()
            page.keyboard.type("docs")
            expect(page.locator(".task")).to_have_count(1)
            page.keyboard.press("Escape")
            expect(page.locator(".task")).to_have_count(3)
            screenshot = os.environ.get("OVERVIEW_SCREENSHOT")
            if screenshot:
                page.screenshot(path=screenshot, full_page=True)
            # Expired data leaves the Done filter even with automatic requests off.
            page.uncheck("#auto")
            page.clock.install()
            requests_before_idle = len(api_requests)
            page.clock.fast_forward(125000)
            assert len(api_requests) == requests_before_idle
            expect(page.locator(".task").nth(1)).to_contain_text("Последние известные данные")
            page.click('[data-filter="done"]')
            expect(page.locator(".task")).to_have_count(0)
            page.click('[data-filter="all"]')
            # No mutation of group color/collapse/order just by opening or refreshing the overview.
            state = worker.evaluate("async ids => Promise.all(ids.map(id=>chrome.tabGroups.get(id)))", group_ids)
            assert [g["color"] for g in state] == ["purple", "green", "blue"]
            assert all(g["collapsed"] for g in state)
            count_before = worker.evaluate("async () => (await chrome.tabs.query({})).length")
            page.locator(".task").first.focus()
            page.keyboard.press("ArrowDown")
            expect(page.locator(".task").nth(1)).to_be_focused()
            page.keyboard.press("Enter")
            page.wait_for_timeout(300)
            active = worker.evaluate("async windowId => (await chrome.tabs.query({windowId,active:true}))[0].id", window_id)
            assert active == tab_ids[2]
            assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == count_before
            page.bring_to_front()
            # Read failure must retain and label last-good data, not silently show green.
            fail_auth = True
            page.wait_for_timeout(5100)  # intentional manual-refresh cooldown
            page.click("#refresh")
            expect(page.locator("#refresh")).to_be_enabled(timeout=20000)
            expect(page.locator(".task").first).to_contain_text("Нужен вход")
            expect(page.locator(".task").first).to_contain_text("Последние известные данные")
            page.click('[data-filter="done"]')
            expect(page.locator(".task")).to_have_count(0)
            # Layout remains usable in a narrow standalone window.
            page.click('[data-filter="all"]')
            page.set_viewport_size({"width": 480, "height": 800})
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
        assert not errors, errors
        print(json.dumps({"ok": True, "statuses_enabled": enabled, "api_requests": len(api_requests)}))
    finally:
        context.close()


with tempfile.TemporaryDirectory(prefix="task-overview-smoke-") as directory, sync_playwright() as playwright:
    root = Path(directory)
    run(playwright, root / "no-permissions", False)
    run(playwright, root / "with-permissions", True)
