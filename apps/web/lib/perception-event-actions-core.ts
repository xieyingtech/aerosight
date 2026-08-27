export const PERCEPTION_EVENT_ACTIONS = ["confirm","false_positive","category_correction","assign","investigate","dismiss","resolve"] as const;
export type PerceptionEventAction = typeof PERCEPTION_EVENT_ACTIONS[number];

export function planPerceptionEventAction(input:{
  action:PerceptionEventAction;currentStatus:string;actualVersion:number;expectedVersion:number;
  permissions:ReadonlySet<string>;actorUserId:number;category?:string;
}){
  if(!input.permissions.has("event:handle"))throw new Error("PROJECT_ACCESS_DENIED");
  if(input.actualVersion!==input.expectedVersion)throw new Error("PERCEPTION_EVENT_VERSION_CONFLICT");
  if(["resolved","dismissed"].includes(input.currentStatus))throw new Error("PERCEPTION_EVENT_TERMINAL");
  const targetStatus:Record<PerceptionEventAction,string>={
    confirm:"acknowledged",false_positive:"dismissed",category_correction:"investigating",
    assign:input.currentStatus,investigate:"investigating",dismiss:"dismissed",resolve:"resolved"
  };
  if(input.action==="category_correction"&&!input.category?.trim())throw new Error("EVENT_CATEGORY_REQUIRED");
  return {
    status:targetStatus[input.action],stateVersion:input.actualVersion+1,
    assignedUserId:input.action==="assign"?input.actorUserId:undefined,
    feedbackAction:input.action,
    feedbackValue:input.action==="category_correction"?{category:input.category!.trim()}:{}
  };
}

export function availablePerceptionEventActions(status:string,permissions:ReadonlySet<string>):PerceptionEventAction[]{
  return permissions.has("event:handle")&&!["resolved","dismissed"].includes(status)?[...PERCEPTION_EVENT_ACTIONS]:[];
}
