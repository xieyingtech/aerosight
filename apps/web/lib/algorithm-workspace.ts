export type AlgorithmAsset = {id:number;kind:string;mimeType:string|null;name?:string|null;capturedAt:string|null;createdAt:string;sourceDescription?:string|null};
export function assetName(asset: AlgorithmAsset) {return asset.name?.trim() || `${asset.mimeType?.startsWith('image/') ? '图片' : '素材'} #${asset.id}`;}
export function eligibleAlgorithmAssets(assets:AlgorithmAsset[],imageOnly:boolean) {return assets.filter(a=>!imageOnly || a.mimeType?.startsWith('image/') || a.mimeType === 'video/mp4');}
export type Detection = {detectionKey:string;label:string;confidence:number;pixelGeometry?:{type:string;x:number;y:number;width:number;height:number}};
export function runDetections(canonical:Record<string,unknown>):Detection[] {
 const result=canonical.result as Record<string,unknown>|undefined;
 if(!result || !Array.isArray(result.detections))return [];
 return result.detections.filter((v):v is Detection=>!!v && typeof v==='object' && typeof v.label==='string' && typeof v.confidence==='number' && Number.isFinite(v.confidence));
}
export function validBox(d:Detection){const b=d.pixelGeometry;return !!b&&b.type==='bbox'&&[b.x,b.y,b.width,b.height].every(Number.isFinite)&&b.width>0&&b.height>0;}
export function runStatus(status:string){return ({succeeded:'成功',failed:'失败',timed_out:'超时',running:'运行中',queued:'排队中',polling:'等待结果',waiting_callback:'等待回调',cancelled:'已取消'} as Record<string,string>)[status]??status;}
export function formatDuration(ms:number|null){return ms===null?'—':ms<1000?`${Math.round(ms)} ms`:`${(ms/1000).toFixed(2)} 秒`;}
