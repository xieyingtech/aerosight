import type { ComplianceValue } from "./mission-preflight.ts";

export const aviationComplianceFields = [
  "realNameRegistration",
  "remoteIdentification",
  "operationApproval",
  "takeoffConfirmation",
  "responsibleOperator",
  "incidentReport"
] as const;

export type AviationComplianceField = typeof aviationComplianceFields[number];

export type DeviceAviationCompliance = {
  registrationNumber?: string | null;
  registrationValidUntil?: Date | null;
  remoteIdentificationCode?: string | null;
  responsibleUserId?: number | null;
};

export type RunAviationCompliance = {
  operationApprovalReference?: string | null;
  operationApprovalValidUntil?: Date | null;
  takeoffConfirmedAt?: Date | null;
  takeoffConfirmedByUserId?: number | null;
  responsibleUserId?: number | null;
  incidentReportReference?: string | null;
  incidentReportedAt?: Date | null;
};

function value(reference?: string | null, validUntil?: Date | null): ComplianceValue | undefined {
  const normalized = reference?.trim();
  if (!normalized) return undefined;
  return { reference: normalized, ...(validUntil ? { validUntil } : {}) };
}

export function aviationComplianceSnapshot(
  device: DeviceAviationCompliance,
  run: RunAviationCompliance
): Record<AviationComplianceField, ComplianceValue | undefined> {
  const responsibleUserId = run.responsibleUserId ?? device.responsibleUserId;
  const takeoffConfirmation = run.takeoffConfirmedAt && run.takeoffConfirmedByUserId
    ? `user:${run.takeoffConfirmedByUserId}@${run.takeoffConfirmedAt.toISOString()}`
    : undefined;
  const incidentReport = run.incidentReportReference && run.incidentReportedAt
    ? `${run.incidentReportReference}@${run.incidentReportedAt.toISOString()}`
    : undefined;

  return {
    realNameRegistration: value(device.registrationNumber, device.registrationValidUntil),
    remoteIdentification: value(device.remoteIdentificationCode),
    operationApproval: value(run.operationApprovalReference, run.operationApprovalValidUntil),
    takeoffConfirmation: value(takeoffConfirmation),
    responsibleOperator: value(responsibleUserId ? `user:${responsibleUserId}` : undefined),
    incidentReport: value(incidentReport)
  };
}
