"""Real MV3 + unresponsive renderer, progressive reads, and the agreed compact layout.
All sites are local fixtures in a disposable Brave profile; no production access.
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

with tempfile.TemporaryDirectory(prefix="overview-resilience-") as directory, sync_playwright() as p:
    directory = Path(directory)
    extension = directory / "extension"
    shutil.copytree(ROOT, extension)
    manifest = json.loads((extension / "manifest.json").read_text())
    manifest["permissions"] += manifest.pop("optional_permissions")
    manifest["host_permissions"] = manifest.pop("optional_host_permissions")
    (extension / "manifest.json").write_text(json.dumps(manifest))
    background = extension / "background.js"
    background.write_text(background.read_text() + "\nglobalThis.resilienceOverview = overview;\n")
    context = p.chromium.launch_persistent_context(
        str(directory / "profile"), executable_path=BINARY, headless=True,
        viewport={"width": 1440, "height": 900},
        args=[f"--disable-extensions-except={extension}", f"--load-extension={extension}"],
    )
    calls = []
    def respond(route):
        url = route.request.url
        if "/api/" in url:
            calls.append(url)
            if url.endswith("/approvals"):
                body = {"approved": True, "approved_by": [{"user": {"id": 1, "name": "Ревьюер"}}]}
            elif "/cards/" in url:
                body = {"id": int(url.rsplit("/", 1)[-1]), "state": 2, "column": {"title": "В разработке"}, "blocked": False,
                        "members": [{"type": 2, "full_name": "Исполнитель"}]}
            else:
                body = {"iid": int(url.rsplit("/", 1)[-1]), "state": "opened", "head_pipeline": {"status": "success"},
                        "has_conflicts": False, "blocking_discussions_resolved": True}
            route.fulfill(content_type="application/json", body=json.dumps(body))
        else:
            route.fulfill(content_type="text/html; charset=utf-8", body="<meta charset='utf-8'><title>ATS-1234 fix: Проверка обновления</title>")
    context.route(re.compile(r"^https?://"), respond)
    try:
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker")
        extension_id = worker.url.split("/")[2]
        sources = []
        ids = []
        for url in ["https://kaiten.x5.ru/space/1/card/99", *[f"https://scm.x5.ru/team/app/-/merge_requests/{i}" for i in [41, 42, 43]]]:
            with context.expect_page() as created:
                tab_id = worker.evaluate("async () => (await chrome.tabs.create({url:'about:blank'})).id")
            created.value.goto(url)
            sources.append(created.value)
            ids.append(tab_id)
        window_id = worker.evaluate("async ids => { const g=await chrome.tabs.group({tabIds:ids}); await chrome.tabGroups.update(g,{color:'purple',collapsed:true}); return (await chrome.tabs.get(ids[0])).windowId; }", ids)
        page = context.new_page()
        page.goto(f"chrome-extension://{extension_id}/overview.html")
        page.select_option("#window", str(window_id))
        expect(page.locator("#refresh")).to_be_enabled(timeout=5000)
        expect(page.locator(".task")).to_contain_text("В разработке")
        page.uncheck("#auto")
        # Pause the Kaiten renderer, not the worker or overview. The in-page fetch timer
        # cannot run here. Page.setWebLifecycleState(frozen) is insufficient: Chromium
        # may thaw it when injecting, so use a genuinely unresponsive execution context.
        worker.evaluate("() => {globalThis.resilienceOverview.cache={}}")
        paused = context.new_cdp_session(sources[0])
        paused.send("Debugger.enable")
        paused.send("Debugger.pause")
        try:
            page.click("#refresh")
            # Fast GitLab results must reach the UI without waiting for Kaiten.
            expect(page.locator("#data-health")).to_contain_text("3 из 4", timeout=3000)
            expect(page.locator("#refresh")).to_be_enabled(timeout=22000)
            inventory = worker.evaluate("async () => globalThis.resilienceOverview.snapshot(false,false)")
            card = inventory["groups"][0]["resources"][0]
            assert card["error"]["code"] == "timeout", card
            print("PASS: paused renderer times out; other statuses appear progressively")
        finally:
            paused.send("Debugger.resume")
            paused.send("Debugger.disable")
        # The original proposal: a compact summary row, separate MR and CI columns.
        assert page.locator("#column-head").inner_text().splitlines() == ["Задача", "Kaiten", "Merge requests", "Ревью", "CI"]
        row_height = page.locator(".task").bounding_box()["height"]
        assert row_height <= 84, f"summary row is {row_height}px tall for a task with 3 MRs"
        assert page.locator(".task-details[open]").count() == 0
        page.locator(".task-details > summary").click()
        expect(page.locator(".resource-link")).to_have_count(4)
        assert page.locator(".task-details[open]").count() == 1
        print("PASS: compact five-column summary; per-resource links expand on demand")
        if os.environ.get("OVERVIEW_SCREENSHOT"):
            page.locator(".task-details > summary").click()
            page.screenshot(path=os.environ["OVERVIEW_SCREENSHOT"], full_page=True)
    finally:
        context.close()
