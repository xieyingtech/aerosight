export type DependencyName = "database" | "object_storage" | "algorithm_service" | "model_service" | "device_adapter";
export type DependencyState = "available" | "unavailable" | "disabled";

export type DependencyHealth = Record<DependencyName, DependencyState>;

export type ProjectHealth = {
  status: "healthy" | "degraded" | "unavailable";
  ready: boolean;
  historicalDataAvailable: boolean;
  degradationReasons: string[];
  capabilityAvailability: Record<string, "available" | "degraded" | "disabled">;
};

const reasonCodes: Record<DependencyName, string> = {
  database: "DATABASE_UNAVAILABLE",
  object_storage: "OBJECT_STORAGE_UNAVAILABLE",
  algorithm_service: "ALGORITHM_SERVICE_UNAVAILABLE",
  model_service: "MODEL_SERVICE_UNAVAILABLE",
  device_adapter: "DEVICE_ADAPTER_UNAVAILABLE"
};

export function dependencyHealthFromRecord(value: unknown): DependencyHealth {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const state = (name: DependencyName): DependencyState => {
    const candidate = record[name];
    return candidate === "unavailable" || candidate === "disabled" || candidate === "available" ? candidate : "available";
  };
  return {
    database: state("database"),
    object_storage: state("object_storage"),
    algorithm_service: state("algorithm_service"),
    model_service: state("model_service"),
    device_adapter: state("device_adapter")
  };
}

export function evaluateProjectHealth(dependencies: DependencyHealth): ProjectHealth {
  const degradationReasons = (Object.entries(dependencies) as Array<[DependencyName, DependencyState]>)
    .filter(([, state]) => state === "unavailable")
    .map(([name]) => reasonCodes[name]);
  const databaseAvailable = dependencies.database === "available";
  const capability = (dependency: DependencyName) => dependencies[dependency] === "available"
    ? "available" as const : dependencies[dependency] === "disabled" ? "disabled" as const : "degraded" as const;
  return {
    status: !databaseAvailable ? "unavailable" : degradationReasons.length ? "degraded" : "healthy",
    ready: databaseAvailable,
    historicalDataAvailable: databaseAvailable,
    degradationReasons,
    capabilityAvailability: {
      historical_queries: databaseAvailable ? "available" : "degraded",
      media_ingestion: capability("object_storage"),
      algorithm_execution: capability("algorithm_service"),
      ai_generation: capability("model_service"),
      realtime_device_control: capability("device_adapter")
    }
  };
}
