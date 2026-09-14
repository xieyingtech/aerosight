import assert from "node:assert/strict";
import test from "node:test";
import { editTaskField, readTaskSource, switchTaskFormat, type TaskSource } from "./task-source-editor.ts";

test("YAML/form/JSON changes preserve custom steps and nested parameters", () => {
  const initial: TaskSource = {format:"yaml",source:"# 巡检配置\napiVersion: aerosight/v2\nname: old\ntrigger: {type: schedule, cron: '0 8 * * *', timezone: Asia/Shanghai}\nsteps:\n  - key: custom\n    uses: inspection.detect\n    with:\n      custom: {nested: [1, 2]}\n"};
  const edited=editTaskField(initial,["name"],"new");
  assert.match(edited.source,/# 巡检配置/);
  const json=switchTaskFormat(edited,"json");
  const back=switchTaskFormat(json,"yaml");
  assert.deepEqual(readTaskSource(back),readTaskSource(edited));
  assert.equal(readTaskSource(back).name,"new");
  assert.deepEqual((readTaskSource(back).steps as Array<Record<string,unknown>>)[0].with,{custom:{nested:[1,2]}});
});

test("invalid YAML remains untouched when switching or editing fails",()=>{
  const invalid:TaskSource={format:"yaml",source:"name: [unfinished"};
  assert.throws(()=>switchTaskFormat(invalid,"json"));
  assert.throws(()=>editTaskField(invalid,["name"],"replacement"));
  assert.equal(invalid.source,"name: [unfinished");
  assert.throws(()=>readTaskSource({format:"yaml",source:"name: a\nname: b"}));
});
