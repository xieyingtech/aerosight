import type {Detection} from './algorithm-workspace';
export type VideoAnnotationFrame={index:number;timeMs:number;width:number;height:number;result:{kind:string;detections?:Detection[];[key:string]:unknown}};
export function videoFrameAt(frames:VideoAnnotationFrame[],timeMs:number):VideoAnnotationFrame|undefined {
 let low=0,high=frames.length-1,selected=-1;
 while(low<=high){const middle=(low+high)>>>1;if(frames[middle].timeMs<=timeMs){selected=middle;low=middle+1;}else high=middle-1;}
 return selected<0?undefined:frames[selected];
}
