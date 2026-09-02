import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { authorizeFlightHubManagementRead, flightHubJoinCodeLookupSchema,
  presentFlightHubManagementResources, presentScopedFlightHubJoinCode, readFlightHubManagementCore } from "./dji-flighthub-management-core.ts";

const allowed={role:"owner",connectorProjectId:11,connectorTeamId:7,teamId:7,connectorStatus:"connected",managementCapabilityVerified:true};

test("management read requires local manager and independent upstream management evidence",()=>{
  authorizeFlightHubManagementRead(11,allowed);
  assert.throws(()=>authorizeFlightHubManagementRead(11,{...allowed,role:"member"}),/PERMISSION_DENIED/);
  assert.throws(()=>authorizeFlightHubManagementRead(11,{...allowed,managementCapabilityVerified:false}),/CAPABILITY_REQUIRED/);
  assert.throws(()=>authorizeFlightHubManagementRead(11,{...allowed,connectorProjectId:99}),/SCOPE_MISMATCH/);
});

test("management projection allowlists fields and rejects hidden vendor identity or location",()=>{
  const resources=presentFlightHubManagementResources([{id:"1",connectorId:"8",kind:"project-member",status:"active",lastSeenAt:"2099-01-01T00:00:00Z",missingAt:null,
    summary:{account:"safe@example.invalid",projectRole:"member",online:true,offlinePosition:{latitude:31,longitude:121},controlDeviceSN:"SECRET_SN",vendorUserId:"SECRET_USER"}}]);
  const serialized=JSON.stringify(resources);
  assert.match(serialized,/safe@example\.invalid/);
  assert.doesNotMatch(serialized,/offlinePosition|latitude|longitude|controlDeviceSN|SECRET_SN|vendorUserId|SECRET_USER/);
});

test("join code lookup accepts only bounded identifier input",()=>{
  assert.deepEqual(flightHubJoinCodeLookupSchema.parse({projectCode:"PROJECT-1",fastJoinCode:"JOIN-1"}),{projectCode:"PROJECT-1",fastJoinCode:"JOIN-1"});
  assert.throws(()=>flightHubJoinCodeLookupSchema.parse({projectCode:"PROJECT-1",fastJoinCode:"JOIN-1",targetUrl:"https://evil.invalid"}));
  assert.throws(()=>flightHubJoinCodeLookupSchema.parse({projectCode:"../outside",fastJoinCode:"JOIN-1"}));
  const result={projectUuid:"00000000-0000-4000-8000-000000000001",organizationUuid:"00000000-0000-4000-8000-000000000010",
    projectName:"项目",organizationName:"组织",userInOrganization:true,recommendedUserCallsign:"用户",recommendedDroneCallsign:null};
  assert.equal(presentScopedFlightHubJoinCode({projectUuid:result.projectUuid,organizationUuid:result.organizationUuid},result).projectName,"项目");
  assert.throws(()=>presentScopedFlightHubJoinCode({projectUuid:"00000000-0000-4000-8000-000000000099",organizationUuid:result.organizationUuid},result),/SCOPE_MISMATCH/);
});

test("management API and UI do not expose remote ids, credentials, positions or controlled device serials",()=>{
  const service=readFileSync(new URL("./dji-flighthub-management.ts",import.meta.url),"utf8");
  const route=readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/management/route.ts",import.meta.url),"utf8");
  const component=readFileSync(new URL("../components/dji-flighthub-management-panel.tsx",import.meta.url),"utf8");
  for(const source of [route,component]) assert.doesNotMatch(source,/credential_envelope|remote_id|offline_position|control_device_sn/i);
  const publicProjection=service.slice(service.indexOf("export async function readFlightHubManagement"));
  assert.doesNotMatch(publicProjection,/remote_id|offline_position|control_device_sn/i);
});

test("ordinary project membership stops before any management projection query",async()=>{
  const statements:string[]=[];
  const client={query:async(text:string)=>{statements.push(text);if(text.includes("flighthub-management:access"))return{rows:[{...allowed,role:"member",connectorId:"8",connectorName:"司空"}],rowCount:1};return{rows:[],rowCount:0};},release:()=>{}};
  await assert.rejects(()=>readFlightHubManagementCore(5,11,"8",async()=>client as never),/PERMISSION_DENIED/);
  assert(!statements.some(text=>text.includes("flighthub-management:resources")));
  assert(!statements.some(text=>text.includes("flighthub-management:state")));
});
