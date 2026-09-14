import { stringify } from "yaml";
import type { TaskSource } from "./task-source-editor.ts";

export function inspectionTaskTemplate(mode: "assets" | "existing-flight" | "flighthub-flight"): TaskSource {
  return {format:"yaml",source:stringify({
    apiVersion:"aerosight/v2", name:mode==="assets"?"航拍图片巡检":mode==="existing-flight"?"司空已有飞行巡检":"司空新飞行巡检",
    trigger:{type:"manual"}, concurrencyLimit:1,
    steps:[
      {key:"observe",name:"读取巡检证据",uses:"inspection.observe",with:mode==="assets"?{mode,assetIds:[]}:mode==="existing-flight"?{mode,connectorId:0,flightUuid:""}:{mode,connectorId:0,deviceId:0,waylineResourceId:0,waylineVersion:{},schedulerOwner:"aerosight",taskType:"immediate"}},
      {key:"detect",name:"目标识别",uses:"inspection.detect",dependsOn:["observe"],with:mode==="assets"?{observationId:"steps.observe.outputs.observationId",source:"external",algorithmDefinitionVersionId:0}:{observationId:"steps.observe.outputs.observationId",source:"flighthub-ai"}},
      {key:"assess",name:"证据研判",uses:"copilot.run",dependsOn:["detect"],with:{mode:"assessment",temperature:0.2,evidenceSetId:"steps.detect.outputs.evidenceSetId"}},
      {key:"issue",name:"处理案件",uses:"issue.create-or-update",dependsOn:["assess"],with:{assessmentId:"steps.assess.outputs.assessmentId"}},
      {key:"report",name:"生成报告",uses:"report.generate",dependsOn:["issue"],with:{scope:"current-run"}}
    ]
  })};
}
