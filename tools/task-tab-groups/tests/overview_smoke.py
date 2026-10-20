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
        args=[f"--disable-extensions-except={extension}", f"--load-extension={extension}",
              "--no-proxy-server", "--host-resolver-rules=MAP * ~NOTFOUND"],
    )
    errors = []
    api_requests = []
    fail_auth = False
    fail_review = False
    titles = {
        "https://kaiten.x5.ru/space/1/card/99": "Ускорить проверки | Kaiten",
        "https://scm.x5.ru/team/app/-/merge_requests/42": "ATS-1234 fix: Ускорить проверки",
        "https://kaiten.x5.ru/space/1/card/100": "Обновить контракт | Kaiten",
        "https://scm.x5.ru/team/docs/-/merge_requests/42": "ATS-5678 docs: Обновить контракт",
        "https://scm.x5.ru/team/service/-/merge_requests/18": "feat: Новая интеграция",
    }

    def respond(route):
        url = route.request.url
        if "/api/" in url:
            api_requests.append({"url": url, "method": route.request.method})
            if fail_auth:
                route.fulfill(status=401, content_type="application/json", body="{}")
                return
            if fail_review and url.endswith("/approvals"):
                route.fulfill(status=403, content_type="application/json", body="{}")
                return
            if url.endswith("/approvals"):
                done = "docs" in url
                draft = "service" in url
                body = {"iid": 18 if draft else 42, "approved": done or draft,
                        "approvals_required": 0 if draft else 2, "approvals_left": 0 if done or draft else 1,
                        "approved_by": [{"user": {"id": 99, "username": "SRV.SAMEDI", "name": "Автоматическое ревью"}}]
                        + ([] if draft else [{"user": {"id": 1, "name": "Одобривший ревьюер"}}]
                           + ([{"user": {"id": 2, "name": "Второй ревьюер"}}] if done else []))}
            elif "/cards/" in url:
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
        page.clock.install()
        page.reload()
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.wait_for_selector("#window option", state="attached")
        before_reopen = worker.evaluate("async () => (await chrome.tabs.query({})).length")
        page.evaluate("() => chrome.runtime.sendMessage({type:'overview-open'})")
        assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == before_reopen
        expect(page.locator("#onboarding")).to_contain_text("Подключите окно")
        page.select_option("#window", str(window_id))
        expect(page.locator(".task")).to_have_count(3)
        expect(page.locator("#refresh")).to_be_enabled()
        if not enabled:
            page.locator("#access summary").click()
            expect(page.locator("#enable")).to_be_visible()
            expect(page.locator(".task").first).to_contain_text("Чтение статусов отключено")
            assert not api_requests
        else:
            page.locator("#access summary").click()
            expect(page.locator("#disable")).to_be_visible()
            page.evaluate("() => navigator.clipboard.writeText = async text => {globalThis.copiedDiagnostic = text}")
            page.click("#diagnose")
            report = json.loads(page.locator("#diagnostics").inner_text())
            assert report["sources"]["kaiten"]["fresh"] == 2
            assert report["sources"]["gitlab"]["fresh"] == 3
            assert report["reading"] is False
            assert "ATS" not in json.dumps(report)
            assert json.loads(page.evaluate("globalThis.copiedDiagnostic")) == report
            page.locator("#access summary").click()
            expect(page.locator(".task").first).to_contain_text("CI упал")
            expect(page.locator(".task").nth(1)).to_contain_text("Merged")
            expect(page.locator(".task").nth(2)).to_contain_text("Draft")
            assert len(api_requests) == 8, api_requests
            assert report["review"] == {"pending": 1, "approved": 1, "not-required": 1}
            expect(page.locator(".review-summary").first).to_contain_text("Ждёт одобрений")
            expect(page.locator(".review-summary").nth(1).locator(".summary-line")).to_have_class("summary-line good")
            expect(page.locator(".review-summary").nth(2)).to_contain_text("Нет одобрений")
            expect(page.locator(".review-summary").nth(2)).to_contain_text("Автоодобрение не учитывается")
            assert "good" not in page.locator(".review-summary").nth(2).locator(".summary-line").get_attribute("class")
            expect(page.locator(".review-summary").nth(1)).to_contain_text("Одобрений: 2")
            page.locator(".review-summary").nth(2).click()
            expect(page.locator(".task").nth(2)).to_contain_text("Одобрение @SRV.SAMEDI не учитывается.")
            expect(page.locator(".task").nth(2)).not_to_contain_text("Одобрили:")
            page.locator(".review-summary").nth(2).click()
            assert all(request["method"] == "GET" for request in api_requests)
            page.locator(".review-summary").first.click()
            expect(page.locator(".task").first.locator(".resource-link").first).to_be_visible()
            expect(page.locator(".task").first).to_contain_text("Одобрили: Одобривший ревьюер")
            expect(page.locator(".task").first).not_to_contain_text("Автоматическое ревью")
            expect(page.locator(".task").first).to_contain_text("Одобрение @SRV.SAMEDI не учитывается.")
            expect(page.locator(".task").first).to_contain_text("Назначены на ревью: Тестовый ревьюер")
            expect(page.locator(".task").first).to_contain_text("Нужно ещё одобрений: 1")
            page.clock.run_for(15000)
            expect(page.locator(".task-details[open]")).to_have_count(1)
            page.locator(".mr-summary").first.click()
            expect(page.locator(".task-details[open]")).to_have_count(0)
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
            # Review API failure must not discard a successfully read MR or pipeline,
            # or keep the previous green approval. The error is visible separately.
            fail_review = True
            page.wait_for_timeout(5100)
            page.click("#refresh")
            expect(page.locator("#refresh")).to_be_enabled(timeout=20000)
            expect(page.locator(".review-summary").nth(1)).to_contain_text("Ревью неизвестно")
            assert "good" not in page.locator(".review-summary").nth(1).locator(".summary-line").get_attribute("class")
            expect(page.locator(".ci-summary").nth(1).locator(".summary-line")).to_have_class("summary-line good")
            expect(page.locator("#data-health")).to_contain_text("ревью без данных: 3")
            page.locator(".review-summary").nth(1).click()
            expect(page.locator(".task").nth(1)).to_contain_text("Ревью: Нет доступа к API")
            page.locator(".review-summary").nth(1).click()
            # Expired data leaves the Done filter even with automatic requests off.
            page.uncheck("#auto")
            requests_before_idle = len(api_requests)
            page.clock.fast_forward(125000)
            assert len(api_requests) == requests_before_idle
            # Tab events still update inventory, but cannot trigger site reads with auto off.
            worker.evaluate("async id => chrome.tabGroups.update(id,{title:'Событие группы 🔒'})", group_ids[2])
            page.clock.run_for(1000)
            expect(page.locator(".task").nth(2)).to_contain_text("Событие группы")
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
            expect(page.locator(".task").first).to_contain_text("API не принял авторизацию")
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
