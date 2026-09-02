import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { authorizeFlightHubDeviceAdmin, flightHubDeviceAdminInputSchema } from "./dji-flighthub-device-admin-core.ts";

const input = flightHubDeviceAdminInputSchema.parse({ connectorInstanceId: 8, idempotencyKey: "rtk-00001", approvalRequestId: "00000000-0000-4000-8000-000000000008",
  action: "rtk-calibrate", deviceId: 42, confirmation: "CALIBRATE RTK",
  request: { host: "ntrip.invalid", port: 8002, account: "account", password: "password", mountPoint: "mount" } });
const allowed = { teamId: 7, role: "owner", connectorProjectId: 11, connectorTeamId: 7, connectorStatus: "connected",
  featureEnabled: true, capabilityVerified: true, deviceProjectId: 11, identityPresent: true, deviceOnline: true, stateFresh: true,
  approvalProjectId: 11, approvalTeamId: 7, approvalResourceType: "device", approvalResourceId: "42",
  approvalAction: "flighthub.admin.rtk-calibrate", approvalStatus: "approved", approvalUnexpired: true };

test("device admin actions default closed behind owner, flag, field evidence and exact approval", () => {
  assert.equal(authorizeFlightHubDeviceAdmin(11,input,allowed).capability,"device.rtk.calibrate");
  for(const override of [{role:"member"},{featureEnabled:false},{capabilityVerified:false},{deviceOnline:false},{stateFresh:false},
    {approvalAction:"flighthub.admin.relay-pair"},{approvalResourceId:"99"},{approvalUnexpired:false}]){
    assert.throws(()=>authorizeFlightHubDeviceAdmin(11,input,{...allowed,...override}));
  }
});

test("sensitive inputs require exact confirmation and reject extra fields", () => {
  assert.throws(()=>flightHubDeviceAdminInputSchema.parse({...input,confirmation:"YES"}));
  assert.throws(()=>flightHubDeviceAdminInputSchema.parse({...input,request:{...input.request,extra:"forbidden"}}));
  assert.throws(()=>flightHubDeviceAdminInputSchema.parse({connectorInstanceId:8,idempotencyKey:"decrypt1",approvalRequestId:"00000000-0000-4000-8000-000000000008",action:"sn-decrypt",confirmation:"DECRYPT SN",request:{encryptedSNs:[]}}));
});

test("each device admin action has an independent confirmation and capability", () => {
  const cases = [
    { action:"rtk-calibrate", confirmation:"CALIBRATE RTK", deviceId:42, request:{host:"ntrip.invalid",port:8002,account:"a",password:"p",mountPoint:"m"}, capability:"device.rtk.calibrate" },
    { action:"relay-pair", confirmation:"PAIR RELAY", deviceId:42, request:{pairEnable:true,pairType:"drone"}, capability:"device.relay.pair" },
    { action:"active-project-update", confirmation:"MOVE DEVICE", deviceId:42, request:{activeProjectUuid:"PROJECT_TARGET_REDACTED"}, capability:"device.active-project.update" },
    { action:"sn-decrypt", confirmation:"DECRYPT SN", request:{encryptedSNs:["ENCRYPTED_REDACTED"]}, capability:"security.sn.decrypt" }
  ] as const;
  for (const item of cases) {
    const {capability,...actionInput}=item;
    const parsed=flightHubDeviceAdminInputSchema.parse({connectorInstanceId:8,idempotencyKey:`admin-${item.action}`,approvalRequestId:"00000000-0000-4000-8000-000000000008",...actionInput});
    const deviceId="deviceId" in parsed?parsed.deviceId:null;
    const authorization={...allowed,deviceProjectId:deviceId===null?null:11,identityPresent:deviceId!==null,deviceOnline:deviceId!==null,stateFresh:deviceId!==null,
      approvalResourceType:deviceId===null?"connector":"device",approvalResourceId:String(deviceId??8),approvalAction:`flighthub.admin.${item.action}`};
    assert.equal(authorizeFlightHubDeviceAdmin(11,parsed,authorization).capability,capability);
  }
});

test("device admin audit and API responses omit plaintext administrative secrets", () => {
  const route=readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/device-admin-actions/route.ts",import.meta.url),"utf8");
  const service=readFileSync(new URL("./dji-flighthub-device-admin.ts",import.meta.url),"utf8");
  const auditInput=service.slice(service.indexOf("return withAuditedProjectWrite"),service.indexOf("}, async (client)"));
  const publicReturn=service.slice(service.lastIndexOf("return {action:"));
  assert.match(auditInput,/request:\{digest:requestDigest\}/);
  assert.doesNotMatch(auditInput,/password|encryptedSNs|input\.request/);
  assert.doesNotMatch(route,/request_envelope|result_envelope|password|encryptedSNs/i);
  assert.doesNotMatch(publicReturn,/resultEnvelope|requestEnvelope|requestDigest/);
});
