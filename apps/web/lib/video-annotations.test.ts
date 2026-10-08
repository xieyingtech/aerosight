import assert from 'node:assert/strict';
import test from 'node:test';
import {videoFrameAt,type VideoAnnotationFrame} from './video-annotations.ts';
const frames:VideoAnnotationFrame[]=[0,1000,2000].map((timeMs,index)=>({index,timeMs,width:1280,height:720,result:{kind:'detection',detections:[]}}));
test('video overlays select the preceding annotation at boundaries and after seeks',()=>{
 assert.equal(videoFrameAt([],100),undefined);
 assert.equal(videoFrameAt(frames,-1),undefined);
 assert.equal(videoFrameAt(frames,0)?.index,0);
 assert.equal(videoFrameAt(frames,999)?.index,0);
 assert.equal(videoFrameAt(frames,1000)?.index,1);
 assert.equal(videoFrameAt(frames,2500)?.index,2);
 assert.equal(videoFrameAt(frames,500)?.index,0);
});
