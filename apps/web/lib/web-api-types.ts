// Browser DTOs for the Go HTTP API. Timestamps are JSON strings.
import type { buildAlgorithmCatalogEntry } from "./algorithm-catalog-core";
import type { buildAlgorithmRunDiagnostics } from "./algorithm-run-view-core";
import type { PerceptionEventAction } from "./perception-event-actions-core";


export type AIModelConfig = {
  id: string; protocol: "openai-compatible" | "responses" | "anthropic-messages" | "stepfun-realtime";
  capabilities: string[]; enabled: boolean;
};

export type AIProviderView = {
  models: AIModelConfig[]; isRealtimeDefault: boolean;
  realtimeProtocol: "disabled" | "stepfun"; realtimeModelId: string;
  id: string; name: string; providerType: "openai"; baseUrl: string | null; modelId: string;
  enabled: boolean; isDefault: boolean; status: string; health: Record<string, unknown>;
  lastTestedAt: string | null; updatedAt: string;
};

export type AgentSessionView = {
  id: number;
  status: string;
  summary: string | null;
  createdAt: string;
  messages: Array<{ id: number; role: string; content: string; toolCalls: unknown; createdAt: string }>;
};

export type IssueListItem = {
  id: number; number: number; title: string; status: string; priority: string;
  occurrenceCount: number; labels: string[]; hasMapLocation: boolean;
  firstSeenAt: string; lastSeenAt: string; updatedAt: string;
};

export type AlgorithmProviderView = {
  id: string; name: string; providerType: "http-json" | "kserve-v2" | "ogc-processes" | "ai-sdk";
  baseUrl: string; authType: "none" | "bearer" | "api-key-header" | "basic" | "signed";
  allowedHeaders: string[]; timeoutSeconds: number; concurrencyLimit: number; rateLimitPerMinute: number;
  status: string; health: Record<string, unknown>; updatedAt: string;
};


export type AlgorithmCatalogEntry = ReturnType<typeof buildAlgorithmCatalogEntry>;
export type AlgorithmRunView = {
 id:string; status:string; canonicalResult:Record<string,unknown>;
 rawResultObjectKey:string|null; rawResultChecksumSha256:string|null;
 createdAt:string; startedAt:string|null; finishedAt:string|null;
 errorCode:string|null; errorMessage:string|null;
 definitionName:string;providerName:string;providerType:string;
 inputAssetId:number;taskRunId:number|null;deviceId:number|null;externalJobId:string|null;
};
export type AlgorithmRunDetail = {run:AlgorithmRunView;attempts:Record<string,unknown>[];view:ReturnType<typeof buildAlgorithmRunDiagnostics>};
export type IssueDetail = {
 issue:Record<string,unknown>; events:Record<string,unknown>[]; links:Record<string,unknown>[];
 detections:Record<string,unknown>[];assets:Record<string,unknown>[];assignees:Record<string,unknown>[];
 members:Record<string,unknown>[];agents:Record<string,unknown>[];drafts:Record<string,unknown>[];
 feedback:Record<string,unknown>[];qualityStats:Record<string,unknown>[];
 canHandle:boolean;canAssign:boolean;canUseAgent:boolean;
};
export type PerceptionEventDetail = {event:Record<string,unknown>;detections:Record<string,unknown>[];feedback:Record<string,unknown>[];actions:PerceptionEventAction[]};
