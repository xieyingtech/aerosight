"use client";
import {useRef,type RefObject} from 'react';
import {useAPI} from './use-api';

// Preserve position and play/pause state when renewing an expired object URL.
export function useMediaPlayback(path:string|null, video:RefObject<HTMLVideoElement|null>) {
 const access=useAPI<{url:string;expiresAt?:string}>(path);
 const resume=useRef<{time:number;playing:boolean}|null>(null);
 const recovery=useRef(0);
 function reload(){const v=video.current;if(v)resume.current={time:v.currentTime,playing:!v.paused};access.reload();}
 function recover(){if(Date.now()-recovery.current<60000)return false;recovery.current=Date.now();reload();return true;}
 function loaded(){const state=resume.current;if(!state||!video.current)return;resume.current=null;video.current.currentTime=state.time;if(state.playing)void video.current.play().catch(()=>{});}
 return {...access,reload,recover,loaded};
}
