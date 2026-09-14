"use client";
import { useMemo, useState } from "react";
import { editTaskField, readTaskSource, switchTaskFormat, type TaskSource } from "@/lib/task-source-editor";
import { Button } from "@/components/ui/button";

const object = (v: unknown): Record<string,unknown> => v && typeof v === "object" && !Array.isArray(v) ? v as Record<string,unknown> : {};
const inputClass = "w-full rounded-md border bg-background p-2 text-sm";
export function TaskSourceEditor({value,onChange,readOnly=false}:{value:TaskSource;onChange:(v:TaskSource)=>void;readOnly?:boolean}) {
  const [mode,setMode]=useState<"source"|"form">("source");
  const [message,setMessage]=useState("");
  const parsed=useMemo(()=>{try{return {definition:readTaskSource(value),error:""};}catch(e){return {definition:null,error:e instanceof Error?e.message:"配置无效"};}},[value]);
  const definition=parsed.definition;
  const update=(path:Array<string|number>,replacement:unknown)=>{try{onChange(editTaskField(value,path,replacement));setMessage("");}catch(e){setMessage(e instanceof Error?e.message:"修改失败");}};
  const field=(label:string,path:Array<string|number>,current:unknown,kind="text")=><label className="space-y-1 text-sm" key={path.join(".")+":"+JSON.stringify(current)}><span>{label}</span><input type={typeof current==="boolean"?"checkbox":"text"} defaultChecked={typeof current==="boolean"?current:undefined} aria-label={label} className={inputClass} disabled={readOnly} defaultValue={typeof current==="string"||typeof current==="number"?current:JSON.stringify(current??"")} onBlur={e=>{
    if(typeof current==="boolean") update(path,e.target.checked);
    else if(kind==="json"){try{update(path,JSON.parse(e.target.value));}catch{setMessage("数组参数请填写完整 JSON，例如 [1, 2]");}}
    else update(path,kind==="number" && e.target.value!=="" ? Number(e.target.value) : e.target.value);
  }}/></label>;
  const trigger=object(definition?.trigger);
  const steps=Array.isArray(definition?.steps)?definition.steps:[];
  return <div className="space-y-3">
    <div className="flex flex-wrap gap-2">
      <Button type="button" variant={mode==="source"?"default":"outline"} onClick={()=>setMode("source")}>原始编辑</Button>
      <Button type="button" variant={mode==="form"?"default":"outline"} disabled={!definition} onClick={()=>setMode("form")}>参数表单</Button>
      <select aria-label="定义格式" className="rounded border bg-background p-2" disabled={readOnly} value={value.format} onChange={e=>{try{onChange(switchTaskFormat(value,e.target.value as TaskSource["format"]));setMessage("");}catch(e){setMessage(e instanceof Error?e.message:"无法切换格式");}}}>
        <option value="yaml">YAML</option><option value="json">JSON</option>
      </select>
    </div>
    {(parsed.error||message)&&<p role="alert" className="text-sm text-destructive">{parsed.error||message}</p>}
    {mode==="source"?<textarea aria-label={`任务定义 ${value.format.toUpperCase()}`} className="min-h-[420px] w-full rounded-md border bg-background p-3 font-mono text-xs" readOnly={readOnly} value={value.source} onChange={e=>onChange({...value,source:e.target.value})}/>:definition&&<div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-2">
        {field("任务名称",["name"],definition.name)}
        <label className="space-y-1 text-sm"><span>触发方式</span><select aria-label="触发方式" className={inputClass} disabled={readOnly || !["manual","schedule"].includes(String(trigger.type))} value={String(trigger.type)} onChange={e=>update(["trigger"],e.target.value==="schedule"?{...trigger,type:"schedule",cron:"0 8 * * *",timezone:"Asia/Shanghai",enabled:true}:{type:"manual",...(definition.apiVersion==="aerosight/v2"?{inputs:trigger.inputs??{}}:{})})}>
          <option value="manual">手动</option><option value="schedule">定时</option>{!["manual","schedule"].includes(String(trigger.type))&&<option value={String(trigger.type)}>{String(trigger.type)}（使用原始编辑）</option>}
        </select></label>
        {trigger.type==="schedule"&&<>{field("Cron 表达式",["trigger","cron"],trigger.cron)}{field("时区",["trigger","timezone"],trigger.timezone)}</>}
      </div>
      {Object.keys(object(trigger.inputs)).length>0&&<fieldset className="grid gap-3 rounded border p-3 md:grid-cols-2"><legend>触发输入资源</legend>{Object.entries(object(trigger.inputs)).map(([key,current])=>field(`输入 ${key}`,["trigger","inputs",key],current,typeof current==="number"?"number":typeof current==="object"?"json":"text"))}</fieldset>}
      {steps.map((raw,index)=>{const step=object(raw),parameters=object(step.with);return <fieldset className="space-y-3 rounded border p-3" key={String(step.key??index)} disabled={readOnly}>
        <legend>{String(step.name??step.key)} · {String(step.uses)}</legend>
        <div className="grid gap-3 md:grid-cols-2">{Object.entries(parameters).filter(([,current])=>current===null||typeof current!=="object"||Array.isArray(current)).map(([key,current])=>field(key,["steps",index,"with",key],current,typeof current==="number"?"number":Array.isArray(current)?"json":"text"))}</div>
        {Object.values(parameters).some(v=>typeof v==="object"&&!Array.isArray(v))&&<p className="text-xs text-muted-foreground">嵌套参数请使用原始编辑；未编辑的字段会保留。</p>}
      </fieldset>})}
      <p className="text-xs text-muted-foreground">表单与原始编辑共享草稿。自定义步骤、条件和复杂参数请使用原始编辑。</p>
    </div>}
  </div>;
}
