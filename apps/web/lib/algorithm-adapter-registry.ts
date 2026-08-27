import type { AlgorithmAdapterInput } from "@/lib/algorithm-adapter-contract";

export type AlgorithmProviderType = AlgorithmAdapterInput["definition"]["providerType"];
export type AlgorithmAdapterCapability = {
  providerType: AlgorithmProviderType;
  implementationStatus: "enabled" | "unavailable";
  executionModes: readonly ("synchronous" | "asynchronous" | "callback")[];
  supportsPolling: boolean;
  supportsSignedCallbacks: boolean;
  contractVersion: "aerosight.algorithm.input/v1";
  unavailableReason: string | null;
};

const registry = {
  "http-json": {
    providerType: "http-json", implementationStatus: "enabled",
    executionModes: ["synchronous", "asynchronous"], supportsPolling: false,
    supportsSignedCallbacks: false, contractVersion: "aerosight.algorithm.input/v1", unavailableReason: null
  },
  "kserve-v2": {
    providerType: "kserve-v2", implementationStatus: "unavailable", executionModes: [],
    supportsPolling: false, supportsSignedCallbacks: false,
    contractVersion: "aerosight.algorithm.input/v1", unavailableReason: "KServe V2 adapter is not enabled in this release"
  },
  "ogc-processes": {
    providerType: "ogc-processes", implementationStatus: "unavailable", executionModes: [],
    supportsPolling: false, supportsSignedCallbacks: false,
    contractVersion: "aerosight.algorithm.input/v1", unavailableReason: "OGC API Processes adapter is not enabled in this release"
  },
  "ai-sdk": {
    providerType: "ai-sdk", implementationStatus: "unavailable", executionModes: [],
    supportsPolling: false, supportsSignedCallbacks: false,
    contractVersion: "aerosight.algorithm.input/v1", unavailableReason: "AI SDK adapter is not enabled in this release"
  }
} as const satisfies Record<AlgorithmProviderType, AlgorithmAdapterCapability>;

export function listAlgorithmAdapterCapabilities(): AlgorithmAdapterCapability[] {
  return Object.values(registry);
}

export function algorithmAdapterCapability(providerType: AlgorithmProviderType): AlgorithmAdapterCapability {
  return registry[providerType];
}

export function requireEnabledAlgorithmAdapter(providerType: AlgorithmProviderType): AlgorithmAdapterCapability {
  const capability = algorithmAdapterCapability(providerType);
  if (capability.implementationStatus !== "enabled") {
    throw new Error(`ALGORITHM_ADAPTER_UNAVAILABLE:${providerType}`);
  }
  return capability;
}
