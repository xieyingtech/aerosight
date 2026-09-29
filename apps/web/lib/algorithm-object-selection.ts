import type { Detection } from './algorithm-workspace.ts';

export function resolveObjectSelection(all: Detection[], query: Pick<URLSearchParams, 'get' | 'getAll'>) {
 const active=query.get('filtered')==='1';
 const keys=query.getAll('object');
 const reason=(query.get('reason')??'').slice(0,1000);
 if(!active)return {active:false,valid:true,detections:all,reason:''};
 const known=new Set(all.map(d=>d.detectionKey));
 const valid=keys.length<=100 && keys.every(key=>known.has(key));
 const selected=new Set(keys);
 return {active:true,valid,detections:valid?all.filter(d=>selected.has(d.detectionKey)):[],reason};
}

export function objectReviewExport(runId:string,assetId:number,assetVersion:number|null,detections:Detection[],reason:string){
 return {schemaVersion:'aerosight.object-review/v1',algorithmRunId:runId,asset:{assetId,version:assetVersion},selectionReason:reason,reviewStatus:'needs_review',notice:'仅包含检测候选及筛选依据，需要人工核查；不是违规认定。',detections};
}
