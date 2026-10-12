"""Live MV3 smoke test in an isolated Brave profile; all site responses are fixtures."""
import json, os, re, tempfile, time
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
BINARY = os.environ.get("BRAVE_BINARY", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser")

with tempfile.TemporaryDirectory(prefix="task-group-smoke-") as profile, sync_playwright() as p:
    context = p.chromium.launch_persistent_context(
        profile, executable_path=BINARY, headless=True,
        args=[f"--disable-extensions-except={ROOT}", f"--load-extension={ROOT}"],
    )
    try:
        # Intercept every HTTP(S) request: never contact the user's real work sites.
        def respond(route):
            address = route.request.url
            title = "Ускорить тесты | Kaiten" if "kaiten.x5.ru" in address else "ATS-1234 test: проверки (!2) · Merge requests · GitLab"
            route.fulfill(status=200, content_type="text/html; charset=utf-8", body=f"<meta charset=utf-8><title>{title}</title><p>Synthetic test fixture</p>")
        context.route(re.compile(r"^https?://"), respond)
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker", timeout=20000)
        extension_id = worker.url.split("/")[2]
        window_id = worker.evaluate("async () => (await chrome.windows.getAll({windowTypes:['normal']}))[0].id")
        tab_ids = []
        fixture_pages = []
        for address in ['https://kaiten.x5.ru/space/1/card/99', 'https://scm.x5.ru/t/p/-/merge_requests/2']:
            # Attach Playwright BEFORE navigation so the first request is intercepted.
            with context.expect_page() as created:
                tab_id = worker.evaluate("async windowId => (await chrome.tabs.create({windowId,url:'about:blank'})).id", window_id)
            created.value.goto(address)
            fixture_pages.append(created.value)
            tab_ids.append(tab_id)
        fixture = worker.evaluate("""async ({windowId,tabIds}) => {
            const groupId=await chrome.tabs.group({tabIds});
            await chrome.tabGroups.update(groupId,{title:'',color:'purple',collapsed:true});
            const other=await chrome.windows.create({url:'about:blank'});
            const otherGroup=await chrome.tabs.group({tabIds:[other.tabs[0].id]});
            await chrome.tabGroups.update(otherGroup,{title:'Do not change',color:'blue',collapsed:true});
            return {windowId,groupId,otherGroup,tabIds};
        }""", {"windowId":window_id,"tabIds":tab_ids})
        page = context.new_page()
        page.goto(f"chrome-extension://{extension_id}/popup.html")
        page.wait_for_selector("#window option", state="attached", timeout=10000)
        page.select_option("#window", str(fixture["windowId"]))
        page.click("#select")
        page.wait_for_selector(".group h2")
        expect(page.locator(".group h2").filter(has_text="ATS-1234")).to_have_text("ATS-1234 · Ускорить тесты")
        state = worker.evaluate("""async ({groupId,otherGroup,windowId}) => ({
            group:await chrome.tabGroups.get(groupId),
            other:await chrome.tabGroups.get(otherGroup),
            ids:(await chrome.tabs.query({windowId})).filter(t=>t.groupId===groupId).map(t=>t.id),
        })""",fixture)
        assert state["group"]["title"] == "ATS-1234 · Ускорить тесты"
        assert state["group"]["color"] == "purple"
        assert state["group"]["collapsed"] is True
        assert state["other"]["title"] == "Do not change"
        assert state["ids"] == fixture["tabIds"]
        fixture_pages[0].evaluate("document.title='Новое название | Kaiten'")
        for attempt in range(30):
            name = worker.evaluate("async id => (await chrome.tabGroups.get(id)).title", fixture["groupId"])
            if name == 'ATS-1234 · Новое название': break
            time.sleep(0.1)
        assert name == 'ATS-1234 · Новое название', 'Automatic title event did not rename the group'
        page.reload()
        expect(page.locator('.group h2').filter(has_text='ATS-1234')).to_have_text('ATS-1234 · Новое название')
        row=page.locator(".group").filter(has=page.locator("h2",has_text="ATS-1234"))
        row.locator("input").fill("ATS-1234 · Зафиксировано")
        row.locator("button").click()
        expect(page.locator(".group h2").filter(has_text="ATS-1234")).to_have_text("ATS-1234 · Зафиксировано 🔒")
        await_event = worker.evaluate("""async ({groupId}) => {
            const group=await chrome.tabGroups.get(groupId);
            return group.title;
        }""",fixture)
        assert await_event.endswith("🔒")
        print(json.dumps({"ok":True,"extension_id":extension_id,"checks":["native rename","popup selection","automatic title event","membership/order","color/collapse","other window untouched","persistent name lock"]}))
    finally:
        context.close()
