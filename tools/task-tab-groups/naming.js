export const LOCK_MARKER = " 🔒";
const KEY_PATTERN = /\bATS[-‐‑–—](\d+)\b/gi;
const LEADING_TASK = /^\s*(?:Draft:\s*)?(?:(?:feature|feat|fix|bugfix|hotfix|task)[/-])?(\d{7})(?=\D|$)/i;
const AUTH_TITLE = /^(?:x5\s*ключ|sign\s*in(?:\s*[·|—-].*)?|log\s*in|вход(?:\s*[·|—-].*)?|авторизация|access denied|доступ запрещ[её]н)$/i;
const AUTH_PATH = /(?:^|\/)(?:login|signin|sign-in|auth|oauth2?|authorize|sso)(?:\/|$)/i;
const DEFAULT_HOSTS = { kaiten: ["kaiten.x5.ru"], gitlab: ["scm.x5.ru"] };

export function isLocked(title = "") {
  return title.endsWith(LOCK_MARKER);
}

export function lockedTitle(title = "", locked) {
  const clean = isLocked(title) ? title.slice(0, -LOCK_MARKER.length) : title;
  return locked ? clean.trimEnd() + LOCK_MARKER : clean;
}

function taskKeys(text = "") {
  return [...new Set([...text.matchAll(KEY_PATTERN)].map(match => "ATS-" + match[1]))];
}

function urlLikeTitle(title = "") {
  return /^(?:https?:\/\/)?[^\s/]+\.[^\s/]+(?:\/|\?|#|$)/i.test(title.trim());
}

function badGeneratedTitle(title) {
  return /^(?:ATS-\d+|Kaiten #\d+|MR !\d+) · /.test(title) && urlLikeTitle(title.split(" · ").slice(1).join(" · "));
}

function pageInfo(tab, hosts) {
  let url;
  try { url = new URL(tab.url); } catch { return null; }
  if (!["https:", "http:"].includes(url.protocol)) return null;
  const title = (tab.title || "").trim();
  if (AUTH_TITLE.test(title) || AUTH_PATH.test(url.pathname)) return null;
  let kind;
  if (hosts.kaiten.includes(url.hostname)) kind = "kaiten";
  else if (hosts.gitlab.includes(url.hostname) && /\/-\/(?:merge_requests|tree|commit|pipelines)\//.test(url.pathname)) kind = "gitlab";
  else return null;
  return { kind, url, title, usableTitle: Boolean(title) && !urlLikeTitle(title) };
}

function keysForPage(page) {
  const keys = page.usableTitle ? taskKeys(page.title) : [];
  if (page.kind !== "gitlab") return keys;
  // Seven-digit issue prefixes are not MR iids, pipeline ids or arbitrary query values.
  const leading = page.usableTitle && page.title.match(LEADING_TASK);
  const branch = branchPath(page.url);
  const branchNumber = branch.match(LEADING_TASK);
  return [...keys, ...taskKeys(branch), ...(leading ? ["ATS-" + leading[1]] : []), ...(branchNumber ? ["ATS-" + branchNumber[1]] : [])];
}

function cardIds(page) {
  if (page.kind !== "kaiten") return [];
  const fragment = page.url.hash.replace(/^#\??/, "");
  const values = [page.url.searchParams.get("card_id"), page.url.pathname.match(/\/cards?\/(\d+)(?:\/|$)/)?.[1], new URLSearchParams(fragment).get("card_id")];
  return [...new Set(values.filter(value => /^\d+$/.test(value || "")))];
}

function cleanTitle(title, kind) {
  let value = title.replace(/\s*(?:\||·|—|-)\s*(?:Kaiten|Кайтен|GitLab)\s*$/i, "");
  if (kind === "gitlab") {
    value = value.split(/\s*·\s*(?:Merge requests|Мерж-реквесты|Commits|Pipelines)\b/i)[0];
    value = value.replace(/^Draft:\s*/i, "").replace(LEADING_TASK, "");
  }
  value = value.replace(/\s*\(!\d+\)\s*$/, "").replace(KEY_PATTERN, "").replace(/^\s*[:·|—-]+\s*/, "");
  value = value.replace(/^[\s\p{Extended_Pictographic}\p{Emoji_Modifier}\p{Symbol}\p{Punctuation}\uFE0F\u200D]+/u, "");
  value = value.replace(/^(?:feat|fix|test|chore|refactor|perf|docs|build|ci)(?:\([^)]*\))?:\s*/i, "");
  return value.replace(/\s+/g, " ").trim();
}

function shorten(value, limit) {
  const segments = [...new Intl.Segmenter("ru", { granularity: "grapheme" }).segment(value)].map(part => part.segment);
  return segments.length > limit ? segments.slice(0, limit - 1).join("").trimEnd() + "…" : value;
}

function candidateName(page) {
  if (!page.usableTitle) return "";
  const value = cleanTitle(page.title, page.kind);
  return /^(?:home|your work|главная|pipeline #?\d+|kaiten|gitlab)$/i.test(value) ? "" : value;
}

function sharedMrSubject(pages) {
  const names = pages.map(candidateName);
  if (!names.length || names.some(name => !name)) return "";
  const normalise = name => name.toLocaleLowerCase("ru").replace(/\s+/g, " ").trim();
  if (names.every(name => normalise(name) === normalise(names[0]))) return names[0];
  const headings = names.map(name => name.match(/^([^:：]+)[:：]/)?.[1].trim());
  if (headings.every(Boolean) && headings.every(heading => normalise(heading) === normalise(headings[0]))) return headings[0];
  const words = names.map(name => name.split(/\s+/));
  const common = [];
  for (let index = 0; index < words[0].length; index++) {
    if (!words.every(parts => parts[index] && normalise(parts[index]) === normalise(words[0][index]))) break;
    common.push(words[0][index]);
  }
  // A single generic verb or a partial word is not a shared task subject.
  return common.length >= 2 ? common.join(" ").replace(/[\s:：,;—-]+$/, "") : "";
}

function warning(title, keys, reason) {
  const repair = !title || badGeneratedTitle(title);
  return { state: "conflict", keys, reason, repair, suggested: repair ? "⚠ " + keys.join(" / ") : null };
}

// Pure decisions: no site requests, page-content access or browser mutations.
export function inferGroupName({ title = "", tabs = [] }, { hosts = DEFAULT_HOSTS, maxTitleLength = 44 } = {}) {
  if (isLocked(title)) return { state: "locked", suggested: title, keys: taskKeys(title) };
  const pages = tabs.map(tab => pageInfo(tab, hosts)).filter(Boolean);
  const keys = [...new Set(pages.flatMap(keysForPage))];
  if (keys.length > 1) return warning(title, keys, "different-tasks");
  if (keys.length === 1) {
    const key = keys[0];
    const number = key.slice(4);
    const kaitenPages = pages.filter(page => page.kind === "kaiten");
    const matchedCards = kaitenPages.filter(page => cardIds(page).includes(number));
    const candidates = matchedCards.length ? matchedCards : kaitenPages.length === 1 ? kaitenPages : [];
    const kaitenName = candidates.map(candidateName).find(Boolean);
    if (!kaitenName && title.startsWith(key + " · ") && !badGeneratedTitle(title)) return { state: "ready", key, keys, suggested: title };
    const mrName = pages.filter(page => page.kind === "gitlab" && keysForPage(page).includes(key)).map(candidateName).find(Boolean);
    const name = kaitenName || mrName;
    return { state: "ready", key, keys, suggested: name ? key + " · " + shorten(name, maxTitleLength) : key };
  }
  const cards = [...new Set(pages.flatMap(cardIds))];
  if (cards.length > 1) return warning(title, cards.map(id => "Kaiten #" + id), "different-cards");
  if (cards.length === 1) {
    const key = "Kaiten #" + cards[0];
    const name = pages.filter(page => page.kind === "kaiten" && cardIds(page).includes(cards[0])).map(candidateName).find(Boolean);
    // Kaiten's internal id is explicitly labelled, never silently turned into an ATS key.
    const suggested = name ? key + " · " + shorten(name, maxTitleLength) : title.startsWith(key + " · ") && !badGeneratedTitle(title) ? title : key;
    return { state: "fallback", key, keys: [key], reason: "kaiten-card-only", suggested };
  }
  const mrs = pages.filter(page => page.kind === "gitlab" && /\/-\/merge_requests\/(\d+)(?:\/|$)/.test(page.url.pathname));
  if (mrs.length) {
    // MR iids are project-scoped: never collapse the same iid from different projects.
    const unique = [...new Map(mrs.map(page => [page.url.origin + page.url.pathname.match(/^(.*\/-\/merge_requests\/\d+)/)[1], page])).values()];
    const ids = unique.map(page => page.url.pathname.match(/\/-\/merge_requests\/(\d+)/)[1]);
    const projects = new Set(unique.map(page => page.url.origin + page.url.pathname.split("/-/")[0]));
    const subject = unique.length > 1 ? sharedMrSubject(unique) : "";
    if (subject) return { state: "fallback", keys: [], key: subject, reason: "shared-mr-subject", suggested: shorten(subject, maxTitleLength) + " · " + unique.length + " MR" };
    const key = projects.size > 1 ? "MR · " + unique.length + " запроса" : "MR " + ids.map(id => "!" + id).join(" / ");
    const name = unique.length === 1 ? unique.map(candidateName).find(Boolean) : "";
    return { state: "fallback", key, keys: [], reason: "mr-only", suggested: name ? key + " · " + shorten(name, maxTitleLength) : title.startsWith(key + " · ") && !badGeneratedTitle(title) ? title : key };
  }
  return { state: "pending", keys: [], suggested: null };
}

function branchPath(url) {
  const path = url.pathname.match(/\/-\/tree\/(.*)/)?.[1] || "";
  try { return decodeURIComponent(path); } catch { return path; }
}
