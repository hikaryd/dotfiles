import { GroupNamer } from "./controller.js";

const namer = new GroupNamer(chrome);
let timer;
async function schedule(windowId) {
  await namer.ready;
  if (windowId !== undefined && windowId !== namer.state.windowId) return;
  clearTimeout(timer);
  timer = setTimeout(() => namer.updateAutomatically().catch(() => {}), 500);
}

chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (sender.id !== chrome.runtime.id) return false;
  namer.dispatch(message).then(view => respond({ ok: true, view }))
    .catch(error => respond({ ok: false, error: error.message }));
  return true;
});
chrome.tabs.onUpdated.addListener((id, change, tab) => {
  if (change.title !== undefined || change.url !== undefined || change.status === "complete" || change.groupId !== undefined) schedule(tab.windowId);
});
chrome.tabs.onRemoved.addListener((id, info) => schedule(info.windowId));
chrome.tabs.onMoved.addListener((id, info) => schedule(info.windowId));
chrome.tabs.onAttached.addListener((id, info) => schedule(info.newWindowId));
chrome.tabs.onDetached.addListener((id, info) => schedule(info.oldWindowId));
chrome.tabGroups.onCreated.addListener(group => schedule(group.windowId));
chrome.tabGroups.onUpdated.addListener(group => schedule(group.windowId));
chrome.tabGroups.onRemoved.addListener(group => schedule(group.windowId));
chrome.windows.onRemoved.addListener(id => namer.forgetWindow(id).catch(() => {}));
chrome.commands.onCommand.addListener(command => {
  if (command === "refresh-groups") namer.refresh(true).catch(() => {});
});
