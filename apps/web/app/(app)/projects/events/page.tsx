"use client";
import { Suspense, useEffect } from "react";
import { useRouter } from "next/navigation";
import { positiveParam, usePageQuery } from "@/components/static-api-page";
import { projectPageHref } from "@/lib/page-routes";
function Redirect(){
 const router=useRouter();const query=usePageQuery();const id=positiveParam(query);
 const target=id?projectPageHref(id,"issues",Object.fromEntries(new URLSearchParams(query.toString()))):null;
 useEffect(()=>{if(target) router.replace(target);},[target,router]);
 return <p className="p-4 text-sm text-muted-foreground">{id?"正在打开案件列表…":"无法打开此页面，请从项目列表重新进入。"}</p>;
}
export default function EventsPage(){return <Suspense><Redirect/></Suspense>;}
