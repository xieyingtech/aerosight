import assert from "node:assert/strict";
import test from "node:test";
import {materialRows} from "./material-search.ts";
const assets=[
  {id:1,kind:"media",name:"林间道路",mimeType:"video/mp4",capturedAt:null,createdAt:""},
  {id:2,kind:"media",name:"热成像",sourceDescription:"林间",mimeType:"image/png",capturedAt:null,createdAt:""},
  {id:3,kind:"media",name:"航拍",mimeType:"video/mp4",capturedAt:null,createdAt:""}
];
const matches=[3,1,3,99].map((assetId,i)=>({assetId,startMs:i*1000,endMs:(i+1)*1000,description:"",reference:{href:""}}));
test("exact source before name substring then semantic order, with unique material rows and all windows",()=>{
  const rows=materialRows(assets," 林间 ","all",matches);
  assert.deepEqual(rows.map(row=>row.asset.id),[2,1,3]);
  assert.deepEqual(rows.map(row=>row.match),["exact","text","content"]);
  assert.equal(rows[2].segments.length,2);
});
test("type filter applies to text and content; clearing restores all without stale matches",()=>{
  assert.deepEqual(materialRows(assets,"林间","video",matches).map(row=>row.asset.id),[1,3]);
  assert.deepEqual(materialRows(assets,"","all",matches).map(row=>[row.asset.id,row.segments.length]),[[1,0],[2,0],[3,0]]);
  assert.equal(materialRows(assets,"missing","all",[]).length,0);
});
