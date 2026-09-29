import assert from 'node:assert/strict';
import test from 'node:test';
import {resolveObjectSelection,objectReviewExport} from './algorithm-object-selection.ts';
const all=[{detectionKey:'0',label:'bus',confidence:.9},{detectionKey:'1',label:'person',confidence:.8}];
test('selection restores exact keys, including an explicit empty selection',()=>{
 assert.deepEqual(resolveObjectSelection(all,new URLSearchParams('filtered=1&object=0')).detections,[all[0]]);
 assert.deepEqual(resolveObjectSelection(all,new URLSearchParams('filtered=1')).detections,[]);
 assert.deepEqual(resolveObjectSelection(all,new URLSearchParams()).detections,all);
});
test('invalid selection never silently shows all boxes',()=>{
 const result=resolveObjectSelection(all,new URLSearchParams('filtered=1&object=missing'));
 assert.equal(result.valid,false);assert.deepEqual(result.detections,[]);
 const exported=objectReviewExport('run',1,2,[], '没有候选匹配');
 assert.equal(exported.reviewStatus,'needs_review');assert.equal(exported.asset.version,2);assert.deepEqual(exported.detections,[]);
});
