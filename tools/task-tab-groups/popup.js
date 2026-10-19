import { lockedTitle, isLocked } from "./naming.js";
const $ = id => document.getElementById(id);
const colors = { grey: "#9aa0a6", blue: "#8ab4f8", red: "#f28b82", yellow: "#fdd663", green: "#81c995", pink: "#ff8bcb", purple: "#c58af9", cyan: "#78d9ec", orange: "#fcad70" };
const labels = { ready: "Распознана задача", locked: "Имя зафиксировано", pending: "Нет номера задачи или страница входа — имя сохранено", conflict: "Разные задачи в группе — требуется проверка", fallback: "Номер ATS не подтверждён — показан явный тип и номер" };
let busy = false;
async function send(message) {
  if (busy) return;
  busy = true;
  document.querySelectorAll("button").forEach(button => button.disabled = true);
  try {
    const response = await chrome.runtime.sendMessage(message);
    if (!response?.ok) throw new Error(response?.error || "Расширение не ответило.");
    render(response.view);
  } catch (error) {
    $("status").textContent = error.message;
    $("status").className = "error";
  } finally {
    busy = false;
    document.querySelectorAll("button").forEach(button => button.disabled = false);
  }
}
function render(view) {
  const previous = Number($("window").value);
  $("window").replaceChildren(...view.windows.map(window => {
    const option = document.createElement("option");
    option.value = window.id;
    option.textContent = "Окно " + window.id + " · " + window.count + " вкладок" + (window.focused ? " · активное" : "");
    return option;
  }));
  const selected = view.windowId ?? (view.windows.some(window => window.id === previous) ? previous : view.windows.find(window => window.focused)?.id ?? view.windows[0]?.id);
  $("window").value = selected ?? "";
  $("auto").checked = view.auto;
  $("status").className = view.error ? "error" : "";
  $("status").textContent = view.error || (view.windowId == null ? "Выберите окно. До выбора группы не изменяются." : "Групп: " + view.groups.length + " · выбранное окно " + view.windowId);
  $("groups").replaceChildren(...view.groups.map(group => {
    const row = document.createElement("section");
    row.className = "group";
    row.style.setProperty("--group-color", colors[group.color] || colors.grey);
    const heading = document.createElement("h2");
    heading.textContent = group.title || "Без названия";
    const detail = document.createElement("p");
    detail.textContent = labels[group.state] + " · " + group.count + " вкладок" + (group.state === "conflict" ? " · " + group.keys.join(", ") : "");
    const line = document.createElement("div"); line.className = "line";
    const input = document.createElement("input"); input.type = "text";
    input.setAttribute("aria-label", "Имя группы");
    input.value = lockedTitle(group.title, false);
    const lock = document.createElement("button");
    lock.textContent = isLocked(group.title) ? "Снять 🔒" : "Закрепить 🔒";
    lock.addEventListener("click", () => send({ type: "lock", groupId: group.id, title: input.value, locked: !isLocked(group.title) }));
    line.append(input, lock); row.append(heading, detail, line);
    return row;
  }));
}
$("select").addEventListener("click", () => send({ type: "select", windowId: Number($("window").value) }));
$("refresh").addEventListener("click", () => send({ type: "refresh" }));
$("auto").addEventListener("change", () => send({ type: "auto", enabled: $("auto").checked }));
$("overview").addEventListener("click", async () => {
  try {
    const response = await chrome.runtime.sendMessage({ type: "overview-open" });
    if (!response?.ok) throw new Error(response?.error || "Не удалось открыть обзор.");
    window.close();
  } catch (error) { $("status").textContent = error.message; $("status").className = "error"; }
});
send({ type: "view" });
