import test from "node:test";
import assert from "node:assert/strict";
import { inferGroupName } from "../naming.js";
const mr = (project, title, id=42) => ({url:"https://scm.x5.ru/team/"+project+"/-/merge_requests/"+id,title});
const sameTask=[mr("docs","X5 Career: передача координатора (!42) · GitLab"),mr("app","X5 Career: контракт координатора (!42) · GitLab")];
test("one task may have MRs in docs and app repositories with the same iid",()=>{
 const result=inferGroupName({tabs:sameTask});
 assert.equal(result.state,"fallback");
 assert.equal(result.suggested,"X5 Career · 2 MR");
});
test("a previously generated different-projects name is repaired",()=>{
 assert.equal(inferGroupName({title:"MR 2 · разные проекты",tabs:sameTask}).suggested,"X5 Career · 2 MR");
});
test("a known task key wins over repository count and shared subject",()=>{
 const tabs=sameTask.map(tab=>({...tab,title:"ATS-1234: "+tab.title}));
 const result=inferGroupName({tabs});
 assert.equal(result.state,"ready");
 assert.ok(result.suggested.startsWith("ATS-1234 · "));
});
test("shared wording does not conceal different explicit task numbers",()=>{
 const tabs=[{...sameTask[0],title:"ATS-1234: X5 Career: документация"},{...sameTask[1],title:"ATS-5678: X5 Career: код"}];
 assert.equal(inferGroupName({title:"Моя группа",tabs}).state,"conflict");
});
test("unrelated MR subjects do not become a shared task label",()=>{
 const result=inferGroupName({tabs:[mr("docs","Документация"),mr("app","Платежи")]});
 assert.equal(result.suggested,"MR · 2 запроса");
});
test("word-prefix matching must not join unrelated words",()=>{
 const result=inferGroupName({tabs:[mr("docs","Контракт клиента"),mr("app","Контракты поставщика")]});
 assert.equal(result.suggested,"MR · 2 запроса");
});
test("duplicate tab of the same MR is not counted as a second request",()=>{
 const result=inferGroupName({tabs:[sameTask[0],sameTask[0]]});
 assert.equal(result.suggested,"MR !42 · X5 Career: передача координатора");
});
test("same subject in one repository is handled the same as cross-repository MRs",()=>{
 assert.equal(inferGroupName({tabs:[sameTask[0],mr("docs",sameTask[1].title,43)]}).suggested,"X5 Career · 2 MR");
});
