"use client";
import { Suspense, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { positiveParam } from "@/components/static-api-page";
function Redirect(){
 const router=useRouter();const query=useSearchParams();const id=positiveParam(query);
 useEffect(()=>{if(id) router.replace(`/projects/issues/?${query.toString()}`);},[id,query,router]);
 return <p className="p-4 text-sm text-muted-foreground">{id?"正在打开案件列表…":"无法打开此页面，请从项目列表重新进入。"}</p>;
}
export default function EventsPage(){return <Suspense><Redirect/></Suspense>;}
