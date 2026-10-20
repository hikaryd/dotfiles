"""Brave/MV3 regression: sleeping-tab transport, cookie-based worker GET, mixed CI.
A local HTTPS server stands in for both sites (including service-worker requests).
The card renderer is genuinely paused; the frozen tab metadata is a test seam
because CDP pause/freeze does not set Chrome's tabs.frozen flag in this Brave build.
No production hosts or user profile are contacted. No browser/session credentials.
"""
import json
import os
import ssl
import shutil
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
BINARY = os.environ.get("BRAVE_BINARY", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser")

with tempfile.TemporaryDirectory(prefix="overview-background-") as directory, sync_playwright() as p:
    directory = Path(directory)
    calls = []
    deny = False

    class Site(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            is_api = "/api/" in self.path
            cookie_ok = "fixture_session=logged_in" in self.headers.get("Cookie", "")
            if is_api:
                calls.append({"path": self.path, "cookie": cookie_ok, "method": "GET"})
            if is_api and (not cookie_ok or deny):
                self.send_response(401)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b"{}")
                return
            if self.path.endswith("/approvals"):
                body = json.dumps({"approved": True, "approved_by": [{"user": {"id": 1, "name": "Ревьюер"}}]}).encode()
            elif "/cards/" in self.path:
                card_id = int(self.path.rsplit("/", 1)[-1])
                body = json.dumps({"id": card_id, "state": 2, "column": {"title": "В разработке"}}).encode()
            elif "/merge_requests/" in self.path and is_api:
                iid = int(self.path.rsplit("/", 1)[-1])
                body = json.dumps({"iid": iid, "state": "opened", "head_pipeline": {"status": "success"} if iid == 41 else None}).encode()
            else:
                task = "7654321" if self.path.endswith("/43") else "1234"
                body = f"<meta charset='utf-8'><title>ATS-{task} fix: Карточка из MR</title>".encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json" if is_api else "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    key, cert = directory / "key.pem", directory / "cert.pem"
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", str(key), "-out", str(cert), "-days", "1", "-subj", "/CN=overview-fixture"], check=True, capture_output=True)
    server = ThreadingHTTPServer(("127.0.0.1", 0), Site)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(cert, key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    extension = directory / "extension"
    shutil.copytree(ROOT, extension)
    manifest = json.loads((extension / "manifest.json").read_text())
    manifest["permissions"] += manifest.pop("optional_permissions")
    manifest["host_permissions"] = manifest.pop("optional_host_permissions")
    (extension / "manifest.json").write_text(json.dumps(manifest))
    background = extension / "background.js"
    background.write_text(background.read_text() + """
const nativeGet = chrome.tabs.get.bind(chrome.tabs);
chrome.tabs.get = async id => {
  const tab = await nativeGet(id);
  return id === globalThis.frozenFixtureId ? {...tab, frozen:true} : tab;
};
""")
    port = server.server_port
    context = p.chromium.launch_persistent_context(
        str(directory / "profile"), executable_path=BINARY, headless=True,
        viewport={"width": 1440, "height": 900}, ignore_https_errors=True,
        args=[f"--disable-extensions-except={extension}", f"--load-extension={extension}",
              "--ignore-certificate-errors", "--disable-background-networking", "--no-proxy-server",
              f"--host-resolver-rules=MAP kaiten.x5.ru 127.0.0.1:{port}, MAP scm.x5.ru 127.0.0.1:{port}, MAP * ~NOTFOUND"],
    )
    errors = []
    try:
        context.add_cookies([{"name": "fixture_session", "value": "logged_in", "domain": host,
                             "path": "/", "httpOnly": True, "secure": True, "sameSite": "Strict"}
                            for host in ["kaiten.x5.ru", "scm.x5.ru"]])
        worker = context.service_workers[0] if context.service_workers else context.wait_for_event("serviceworker")
        extension_id = worker.url.split("/")[2]
        ids = []
        sources = []
        for url in ["https://kaiten.x5.ru/space/1/card/99", *[f"https://scm.x5.ru/team/app/-/merge_requests/{i}" for i in [41, 42]]]:
            with context.expect_page() as created:
                tab_id = worker.evaluate("async () => (await chrome.tabs.create({url:'about:blank'})).id")
            created.value.goto(url)
            sources.append(created.value)
            ids.append(tab_id)
        state = worker.evaluate("""async ids => {
          const groupId=await chrome.tabs.group({tabIds:ids});
          await chrome.tabGroups.update(groupId,{color:'purple',collapsed:true});
          return {groupId,windowId:(await chrome.tabs.get(ids[0])).windowId};
        }""", ids)
        page = context.new_page()
        page.goto(f"chrome-extension://{extension_id}/overview.html")
        page.on("pageerror", lambda error: errors.append(str(error)))
        frozen = context.new_cdp_session(sources[0])
        frozen.send("Debugger.enable")
        frozen.send("Debugger.pause")
        worker.evaluate("id => {globalThis.frozenFixtureId = id}", ids[0])
        assert worker.evaluate("async id => (await chrome.tabs.get(id)).frozen", ids[0])
        before = worker.evaluate("async () => (await chrome.tabs.query({})).length")
        page.select_option("#window", str(state["windowId"]))
        expect(page.locator("#refresh")).to_be_enabled(timeout=22000)
        expect(page.locator(".kaiten-summary")).to_contain_text("В разработке")
        expect(page.locator(".ci-summary .summary-line")).to_have_text("✓ Пройден")
        expect(page.locator(".ci-summary .summary-line")).to_have_class("summary-line good")
        expect(page.locator(".ci-summary")).to_contain_text("1 MR без CI")
        page.uncheck("#auto")
        assert worker.evaluate("async id => (await chrome.tabs.get(id)).frozen", ids[0]), "reading must not wake the card"
        assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == before
        group = worker.evaluate("async id => chrome.tabGroups.get(id)", state["groupId"])
        assert group["collapsed"] and group["color"] == "purple"
        assert len(calls) == 5 and all(call["cookie"] and call["method"] == "GET" for call in calls), calls
        print("PASS: MV3 worker reads card with paused renderer + fixture frozen flag using HttpOnly/Secure/SameSite=Strict session; no new tabs or group changes")
        print("PASS: successful pipeline + MR without CI is green with explicit no-CI count")
        if os.environ.get("OVERVIEW_SCREENSHOT"):
            page.screenshot(path=os.environ["OVERVIEW_SCREENSHOT"], full_page=True)
        # Worker transport must show authentication failure, not erase/relabel old data.
        deny = True
        page.wait_for_timeout(5100)  # manual refresh cooldown
        page.click("#refresh")
        expect(page.locator("#refresh")).to_be_enabled(timeout=22000)
        page.locator(".task-details > summary").click()
        expect(page.locator(".resource").first).to_contain_text("API не принял авторизацию")
        expect(page.locator(".kaiten-summary")).to_contain_text("Последние известные данные")
        assert "good" not in page.locator(".ci-summary .summary-line").get_attribute("class")
        assert worker.evaluate("async id => (await chrome.tabs.get(id)).frozen", ids[0])
        print("PASS: background 401 preserves last-known data without fresh/green claims")
        # A group containing only an MR gets a card via the task key, not MR iid.
        deny = False
        with context.expect_page() as created:
            mr_id = worker.evaluate("async () => (await chrome.tabs.create({url:'about:blank'})).id")
        created.value.goto("https://scm.x5.ru/team/app/-/merge_requests/43")
        derived_group = worker.evaluate("""async id => {
            const groupId = await chrome.tabs.group({tabIds:[id]});
            await chrome.tabGroups.update(groupId,{color:'cyan',collapsed:true});
            return groupId;
        }""", mr_id)
        page.bring_to_front()
        row = page.locator(f'.task[data-group-id="{derived_group}"]')
        expect(row).to_be_visible()
        before_derived = worker.evaluate("async () => (await chrome.tabs.query({})).length")
        page.wait_for_timeout(5100)
        page.click("#refresh")
        expect(page.locator("#refresh")).to_be_enabled(timeout=22000)
        expect(row.locator(".kaiten-summary")).to_contain_text("В разработке")
        row.locator(".task-details > summary").click()
        expect(row).to_contain_text("Найдена по номеру задачи в MR")
        assert any(call["path"] == "/api/cards/7654321" and call["cookie"] for call in calls), calls
        assert not any(call["path"].startswith(("/api/v1/cards/", "/api/latest/cards/")) for call in calls), calls
        assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == before_derived
        group = worker.evaluate("async id => chrome.tabGroups.get(id)", derived_group)
        assert group["collapsed"] and group["color"] == "cyan"
        print("PASS: MR-only group reads the inferred card via the web API/session without creating a tab")
        # Navigation is the only point where a missing card may create a tab.
        with context.expect_page() as opened:
            row.locator(".kaiten-summary").click()
        expect(opened.value).to_have_url("https://kaiten.x5.ru/space/18571/boards/card/7654321")
        page.bring_to_front()
        expect(row.locator(".kaiten-summary")).to_contain_text("В разработке")
        row.locator(".kaiten-summary").click()
        expect(page.locator("#error")).to_be_hidden()
        assert worker.evaluate("async () => (await chrome.tabs.query({})).length") == before_derived + 1
        cards = worker.evaluate("async () => chrome.tabs.query({url:'https://kaiten.x5.ru/space/18571/boards/card/7654321'})")
        assert len(cards) == 1 and cards[0]["groupId"] == derived_group, cards
        print("PASS: explicit card click opens it in the MR group; a repeated click reuses it")
        assert not errors, errors
    finally:
        context.close()
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)
