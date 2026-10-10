import type {AlgorithmAsset} from "./algorithm-workspace.ts";

export type MediaType = "all" | "video" | "image";
export type MediaMatch = {assetId:number;startMs:number;endMs:number;description:string;reference:{href:string}};
export type MaterialRow = {asset:AlgorithmAsset;match:"exact"|"text"|"content"|null;segments:MediaMatch[]};
export function mediaType(asset:AlgorithmAsset) {return asset.mimeType?.startsWith("video/") ? "video" : asset.mimeType?.startsWith("image/") ? "image" : "file";}

export function materialRows(assets:AlgorithmAsset[], query:string, filter:MediaType, matches:MediaMatch[]):MaterialRow[] {
  const needle=query.trim().toLocaleLowerCase();
  const eligible=assets.filter(a=>filter === "all" || mediaType(a) === filter);
  if(!needle)return eligible.map(asset=>({asset,match:null,segments:[]}));
  const segments=new Map<number,MediaMatch[]>();
  for(const match of matches){const list=segments.get(match.assetId) ?? [];if(!list.some(m=>m.startMs === match.startMs && m.endMs === match.endMs))list.push(match);segments.set(match.assetId,list);}
  const rows:MaterialRow[]=eligible.flatMap(asset=>{
    const values=[asset.name?.trim() ?? "",asset.sourceDescription?.trim() ?? ""].map(v=>v.toLocaleLowerCase());
    const match=values.includes(needle)?"exact":values.some(v=>v.includes(needle))?"text":null;
    return match?[{asset,match,segments:segments.get(asset.id) ?? []}]:[];
  });
  rows.sort((a,b)=>Number(b.match === "exact")-Number(a.match === "exact"));
  const seen=new Set(rows.map(row=>row.asset.id));
  const byId=new Map(eligible.map(asset=>[asset.id,asset]));
  for(const m of matches){const asset=byId.get(m.assetId);if(asset && !seen.has(asset.id)){seen.add(asset.id);rows.push({asset,match:"content",segments:segments.get(asset.id) ?? []});}}
  return rows;
}
