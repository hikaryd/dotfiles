import test from "node:test";
import assert from "node:assert/strict";
import { inferGroupName } from "../naming.js";
const mr = (title, id=42, project="team/project") => ({url:"https://scm.x5.ru/"+project+"/-/merge_requests/"+id,title});
test("URL-shaped Kaiten title with query parameters must not become a group name",()=>{
 const address="https://kaiten.x5.ru/space/123/boards?card_id=99";
 assert.equal(inferGroupName({tabs:[mr("ATS-3804423: снизить нагрузку"),{url:address,title:address.slice(8)}]}).suggested,"ATS-3804423 · снизить нагрузку");
});
test("an already cached URL-shaped group name must be repaired from the MR",()=>{
 assert.equal(inferGroupName({title:"ATS-3804423 · kaiten.x5.ru/space/123/boards?card_id=99",tabs:[mr("ATS-3804423: снизить нагрузку")]}).suggested,"ATS-3804423 · снизить нагрузку");
});
test("seven-digit task prefix without ATS is a task, not an MR iid",()=>{
 assert.equal(inferGroupName({tabs:[mr("3770723: исправить проверку (!42) · GitLab")]}).suggested,"ATS-3770723 · исправить проверку");
});
test("single Kaiten card uses a clearly labelled internal id, not an invented ATS key",()=>{
 const result=inferGroupName({tabs:[{url:"https://kaiten.x5.ru/space/1/boards?card_id=3815206",title:"Исправить проверку | Kaiten"}]});
 assert.equal(result.state,"fallback");
 assert.equal(result.suggested,"Kaiten #3815206 · Исправить проверку");
});
test("auth return-query card ids are not used",()=>{
 assert.equal(inferGroupName({tabs:[{url:"https://auth.example.com/login?card_id=3815206",title:"X5 Ключ"}]}).state,"pending");
});
test("numberless MRs are labelled as MRs rather than tasks",()=>{
 assert.equal(inferGroupName({tabs:[mr("Исправить проверку",42),mr("Упростить код",43)]}).suggested,"MR !42 / !43");
});
test("MR iids from different projects are not merged into one MR",()=>{
 assert.equal(inferGroupName({tabs:[mr("Описание",42,"team/one"),mr("Описание",42,"team/two")]}).suggested,"Описание · 2 MR");
});
test("feature task conflicts with an explicit ATS key; generated bad name is repaired to a warning",()=>{
 const result=inferGroupName({title:"ATS-3804423 · kaiten.x5.ru/space/1/boards?card_id=9",tabs:[mr("ATS-3804423: исправить"),mr("Feature/3797202 perf (!691)") ]});
 assert.equal(result.state,"conflict");
 assert.equal(result.repair,true);
 assert.equal(result.suggested,"⚠ ATS-3804423 / ATS-3797202");
});
test("conflicts do not overwrite a manually named or locked group",()=>{
 const tabs=[mr("ATS-3804423"),mr("Feature/3797202 perf")];
 assert.equal(inferGroupName({title:"Моя группа",tabs}).suggested,null);
 assert.equal(inferGroupName({title:"Моя группа 🔒",tabs}).state,"locked");
});
test("placeholder card titles cannot poison a last good group title",()=>{
 assert.equal(inferGroupName({title:"ATS-3804423 · Хорошее имя",tabs:[mr("ATS-3804423: другое имя"),{url:"https://kaiten.x5.ru/space/1/boards?card_id=3804423",title:"kaiten.x5.ru/space/1/boards?card_id=3804423"}]}).suggested,"ATS-3804423 · Хорошее имя");
});
