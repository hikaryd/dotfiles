"""One bounded JSON request for the shared Go UI. No REPL, server or shell."""
import contextlib
import html
import json
import re
import shlex
import sys


def item_view(data, last_episode=""):
    description = data.get("description") or ""
    description = re.sub(r"<br\s*/?>", "\n", description, flags=re.I)
    description = html.unescape(re.sub(r"<[^>]*>", "", description)).strip()
    return {
        "title": data["name"]["main"],
        "description": description,
        "year": data.get("year") or 0,
        "kind": (data.get("type") or {}).get("description", ""),
        "count": data.get("episodes_total") or 0,
        "last_episode": last_episode or "",
        "data": data,
    }


def valid_item(item):
    data = item.get("data") if isinstance(item, dict) else None
    return (isinstance(data, dict) and type(data.get("id")) is int
            and data["id"] > 0 and isinstance(data.get("name"), dict)
            and isinstance(data["name"].get("main"), str))


def history_views(entries):
    items = []
    for entry in entries if isinstance(entries, list) else []:
        if not isinstance(entry, dict) or entry.get("extractor_name") != "anilibria":
            continue
        data = entry
        for _ in range(3):
            data = data.get("data") if isinstance(data, dict) else None
        if valid_item({"data": data}):
            items.append(item_view(data, entry.get("episode", "")))
    return items


def player_options(value):
    if not isinstance(value, str) or "\0" in value:
        raise ValueError("invalid mpv options")
    return shlex.split(value)


def validate(request):
    if not isinstance(request, dict) or request.get("op") not in {
        "search", "ongoing", "history", "episodes", "resolve", "remember"
    }:
        raise ValueError("unsupported backend operation")
    op = request["op"]
    if op == "search":
        query = request.get("query")
        if not isinstance(query, str) or not query.strip() or len(query) > 500:
            raise ValueError("enter an anime title (1..500 characters)")
    if op in {"episodes", "remember"} and not valid_item(request.get("item")):
        raise ValueError("invalid anime identity")
    if op in {"resolve", "remember"}:
        episode = request.get("episode")
        if not isinstance(episode, dict) or not isinstance(episode.get("data"), dict):
            raise ValueError("invalid episode")
        if type(episode.get("ordinal")) is not int or not isinstance(episode.get("title"), str):
            raise ValueError("invalid episode identity")
    if op == "resolve":
        player_options(request.get("mpv_opts", ""))


def dispatch(request):
    validate(request)
    # Imports are lazy so malformed requests fail without loading optional tools.
    from anicli_api.source.anilibria import Extractor, Search, Episode
    from anicli.common import history

    op = request["op"]
    if op == "history":
        return {"items": history_views(history.read())}
    with Extractor() as extractor:
        extractor.http.timeout = 30.0
        if op in {"search", "ongoing"}:
            results = extractor.search(request["query"].strip()) if op == "search" else extractor.ongoing()
            return {"items": [item_view(result.data) for result in results]}
        if op in {"episodes", "remember"}:
            data = dict(request["item"]["data"])
            data.setdefault("description", "")
            item = Search(title=data["name"]["main"], thumbnail="", url="_", data=data, **extractor._kwargs_http)
        if op == "episodes":
            episodes = item.get_anime().get_episodes()
            return {"episodes": [{"title": ep.title, "ordinal": ep.ordinal, "data": ep.data} for ep in episodes]}
        data = request["episode"]
        episode = Episode(title=data["title"], ordinal=data["ordinal"], data=data["data"], **extractor._kwargs_http)
        sources = episode.get_sources()
        if op == "remember":
            label = f"{episode.ordinal} {episode.title}"
            history.save(item, "anilibria", {label: sources[0]}, label)
            return {}
        videos = [video for source in sources for video in source.get_videos()]
        return {"videos": [{"quality": video.quality, "url": video.url, "headers": video.headers or {}} for video in videos],
                "args": player_options(request.get("mpv_opts", ""))}


def main():
    try:
        raw = sys.stdin.read((8 << 20) + 1)
        if len(raw) > (8 << 20):
            raise ValueError("backend request exceeded limit")
        request = json.loads(raw)
        with contextlib.redirect_stdout(sys.stderr):
            result = dispatch(request)
        print(json.dumps(result, ensure_ascii=False))
        return 0
    except (ValueError, TypeError, KeyError) as error:
        print(json.dumps({"error": str(error)}, ensure_ascii=False))
    except ImportError:
        print(json.dumps({"error": "anicli runtime incomplete; run ./install -c steps/anicli.yml"}))
    except Exception as error:
        print(json.dumps({"error": f"AniLiberty request failed ({type(error).__name__}); retry or check your connection"}))
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
