import test from "node:test";
import assert from "node:assert/strict";
import { inferGroupName, lockedTitle, isLocked } from "../naming.js";
const mr = (title, path = "/team/project/-/merge_requests/22") => ({ url: "https://scm.x5.ru" + path, title });
const card = title => ({ url: "https://kaiten.x5.ru/space/1/card/99", title });

test("business task number first; Kaiten name wins over the MR title", () => {
  const result = inferGroupName({ tabs: [mr("ATS-3823074 test: ускорить проверки (!22) · Merge requests · GitLab"), card("🔥 🔴 Ускорить тесты | Kaiten")] });
  assert.equal(result.suggested, "ATS-3823074 · Ускорить тесты");
});
test("multiple MRs for the same task are not a conflict", () => {
  assert.equal(inferGroupName({ tabs: [mr("ATS-1234 fix: исправить"), mr("ATS-1234 feat: добавить")] }).state, "ready");
});
test("different task numbers leave the name untouched", () => {
  assert.equal(inferGroupName({ title: "Ручное имя", tabs: [mr("ATS-1234"), mr("ATS-5678")] }).state, "conflict");
});
test("authentication pages and their query strings are not task evidence", () => {
  const tabs = [{ url: "https://scm.x5.ru/users/sign_in?return=ATS-1234", title: "Sign in · GitLab" }, { url: "https://kaiten.x5.ru/space?return=ATS-5678", title: "X5 Ключ" }];
  const result = inferGroupName({ title: "ATS-1234 · Хорошее имя", tabs });
  assert.equal(result.state, "pending");
  assert.equal(result.suggested, null);
});
test("a homepage with URL as title is not used as a task name", () => {
  assert.equal(inferGroupName({ tabs: [mr("ATS-1234 test: ускорить"), { url: "https://kaiten.x5.ru/space", title: "kaiten.x5.ru/space" }] }).suggested, "ATS-1234 · ускорить");
});
test("last good name survives Kaiten authentication while an MR is loaded", () => {
  const result = inferGroupName({ title: "ATS-1234 · Ускорить тесты", tabs: [mr("ATS-1234 test: описание коммита"), { url: "https://kaiten.x5.ru/space", title: "X5 Ключ" }] });
  assert.equal(result.suggested, "ATS-1234 · Ускорить тесты");
});
test("MR iid and Kaiten internal card id are not substituted for a task number", () => {
  const result = inferGroupName({ tabs: [mr("Исправление (!22) · GitLab"), card("Исправление | Kaiten")] });
  assert.equal(result.state, "fallback");
  assert.equal(result.key, "Kaiten #99");
  assert.ok(!result.suggested.includes("ATS-"));
});
test("branch paths may provide a number; namespace paths may not", () => {
  assert.equal(inferGroupName({ tabs: [mr("Исправление", "/team/project/-/tree/ATS-1234")] }).key, "ATS-1234");
  const unrelated = inferGroupName({ tabs: [mr("Исправление", "/ATS-1234/project/-/merge_requests/22")] });
  assert.equal(unrelated.state, "fallback");
  assert.ok(!unrelated.suggested.includes("ATS-1234"));
});
test("unrelated hosts, protocols and standalone page titles are ignored", () => {
  assert.equal(inferGroupName({ tabs: [{ url: "https://example.org", title: "ATS-1234" }, { url: "chrome://newtab", title: "ATS-1234" }] }).state, "pending");
});
test("lock survives new tab metadata and is idempotent", () => {
  const title = lockedTitle("ATS-1234 · Моё имя", true);
  assert.equal(inferGroupName({ title, tabs: [mr("ATS-5678")] }).state, "locked");
  assert.equal(lockedTitle(title, true), title);
  assert.equal(lockedTitle(title, false), "ATS-1234 · Моё имя");
  assert.equal(isLocked(title), true);
});
test("Unicode task keys and digit-leading names remain readable", () => {
  assert.equal(inferGroupName({ tabs: [mr("ats‑1234"), card("123 проверки | Kaiten")] }).suggested, "ATS-1234 · 123 проверки");
});
test("long names are bounded without splitting emoji/graphemes", () => {
  const result = inferGroupName({ tabs: [mr("ATS-1234"), card("Очень ".repeat(30) + "| Kaiten")] });
  assert.ok(result.suggested.endsWith("…"));
  assert.ok([...new Intl.Segmenter("ru", { granularity: "grapheme" }).segment(result.suggested.split(" · ")[1])].length <= 44);
});
