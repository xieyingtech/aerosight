import { createHash } from "node:crypto";

type AuditBoundary<T> = {
  begin: () => Promise<void>;
  writeAudit: () => Promise<number>;
  execute: () => Promise<T>;
  completeAudit: (auditId: number, result: T) => Promise<void>;
  commit: () => Promise<void>;
  rollback: () => Promise<void>;
};

function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, item]) => [key, stableValue(item)])
    );
  }
  return value;
}

export function auditHash(value: unknown) {
  return createHash("sha256").update(JSON.stringify(stableValue(value))).digest("hex");
}

export async function executeWithinAuditBoundary<T>(boundary: AuditBoundary<T>) {
  await boundary.begin();
  try {
    const auditId = await boundary.writeAudit();
    const result = await boundary.execute();
    await boundary.completeAudit(auditId, result);
    await boundary.commit();
    return result;
  } catch (error) {
    await boundary.rollback();
    throw error;
  }
}
