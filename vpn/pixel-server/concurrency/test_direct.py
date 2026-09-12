import json
import unittest
from unittest.mock import Mock, patch

from test_scheduler import r


class DirectTests(unittest.TestCase):
    def test_ids_and_session_routing_ignore_events(self):
        socket = Mock()
        socket.recv.side_effect = [
            json.dumps({"method": "Target.event"}),
            json.dumps({"id": 1, "result": {}}),
            json.dumps({"id": 2, "result": {}}),
        ]
        client = r.DirectCDP(socket)
        client.cdp("Target.getTargets")
        client.meta({"meta": "set_session", "session_id": "owned-session"})
        client.cdp("Page.navigate", url="https://example.org")
        first, second = [
            json.loads(call.args[0]) for call in socket.send.call_args_list
        ]
        self.assertNotIn("sessionId", first)
        self.assertEqual(second["sessionId"], "owned-session")
        self.assertEqual([first["id"], second["id"]], [1, 2])
        self.assertTrue(
            all(call.kwargs["timeout"] > 0 for call in socket.recv.call_args_list)
        )

    def test_unattached_page_and_timeout_fail_closed(self):
        client = r.DirectCDP(Mock())
        with self.assertRaises(RuntimeError):
            client.cdp("Runtime.evaluate")
        client.socket.recv.side_effect = TimeoutError("deadline")
        with self.assertRaises(TimeoutError):
            client.cdp("Target.getTargets")

    def test_discovery_uses_original_origin_not_advertised_host(self):
        connection = Mock()
        response = connection.getresponse.return_value
        response.status = 200
        response.read.return_value = (
            b'{"webSocketDebuggerUrl":"ws://foreign.invalid/devtools/browser/token"}'
        )
        with patch.object(
            r.http.client, "HTTPConnection", return_value=connection
        ) as factory:
            url = r.discover_websocket("http://127.0.0.1:19222")
        self.assertEqual(url, "ws://127.0.0.1:19222/devtools/browser/token")
        factory.assert_called_once_with("127.0.0.1", 19222, timeout=5)
        connection.request.assert_called_once_with("GET", "/json/version")
        connection.close.assert_called_once()

    def test_discovery_does_not_follow_redirect(self):
        connection = Mock()
        connection.getresponse.return_value.status = 302
        with (
            patch.object(r.http.client, "HTTPConnection", return_value=connection),
            self.assertRaises(RuntimeError),
        ):
            r.discover_websocket("http://127.0.0.1:19222")
        self.assertEqual(connection.request.call_count, 1)
        connection.close.assert_called_once()

    def test_android_browser_path_without_id(self):
        connection = Mock()
        response = connection.getresponse.return_value
        response.status = 200
        response.read.return_value = (
            b'{"webSocketDebuggerUrl":"ws://127.0.0.1:19222/devtools/browser"}'
        )
        with patch.object(r.http.client, "HTTPConnection", return_value=connection):
            self.assertEqual(
                r.discover_websocket("http://127.0.0.1:19222"),
                "ws://127.0.0.1:19222/devtools/browser",
            )

    def test_discovery_rejects_lookalike_and_traversal_paths(self):
        for path in (
            "/devtools/browser/",
            "/devtools/browser-evil",
            "/devtools/browser/../page",
            "/devtools/browser/%2e%2e",
            "/devtools/browser/id/other",
            "/devtools/page/id",
        ):
            connection = Mock()
            response = connection.getresponse.return_value
            response.status = 200
            response.read.return_value = json.dumps(
                {"webSocketDebuggerUrl": "ws://127.0.0.1:19222" + path}
            ).encode()
            with (
                patch.object(r.http.client, "HTTPConnection", return_value=connection),
                self.assertRaises(ValueError),
            ):
                r.discover_websocket("http://127.0.0.1:19222")

    def test_explicit_browser_websocket_no_discovery(self):
        for endpoint in (
            "ws://127.0.0.1:19222/devtools/browser",
            "wss://example.org/devtools/browser/safe-id_1",
        ):
            with patch.object(r.http.client, "HTTPConnection") as connection:
                self.assertEqual(r.discover_websocket(endpoint), endpoint)
                connection.assert_not_called()

    def test_invalid_explicit_browser_websockets(self):
        for endpoint in (
            "ws://user:pass@example.org/devtools/browser",
            "ws://example.org/devtools/browser?secret=1",
            "ws://example.org/devtools/browser#fragment",
            "ws://example.org/devtools/page/id",
            "ws://example.org/devtools/browser/../other",
        ):
            with self.assertRaises(ValueError):
                r.discover_websocket(endpoint)
