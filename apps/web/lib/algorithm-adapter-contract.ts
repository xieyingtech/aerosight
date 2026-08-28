import { z } from "zod";

const assetReferenceSchema = z.object({
  assetId: z.number().int().positive(),
  version: z.number().int().positive(),
  checksumSha256: z.string().regex(/^[a-f0-9]{64}$/),
  mimeType: z.string().min(1),
  accessUrl: z.url().refine((value) => value.startsWith("https://"), "asset access URL must use HTTPS"),
  accessExpiresAt: z.iso.datetime()
});

const spatiotemporalContextSchema = z.object({
  capturedAt: z.iso.datetime(),
  deviceId: z.number().int().positive().nullable(),
  taskRunId: z.number().int().positive().nullable(),
  position: z.object({ longitude: z.number().min(-180).max(180), latitude: z.number().min(-90).max(90), altitudeMeters: z.number().optional() }).nullable(),
  coordinateReference: z.string().min(1).nullable(),
  calibrationVersion: z.string().min(1).nullable(),
  quality: z.record(z.string(), z.unknown())
});

export const algorithmAdapterInputSchema = z.object({
  schemaVersion: z.literal("aerosight.algorithm.input/v1"),
  runId: z.uuid(),
  projectId: z.number().int().positive(),
  definition: z.object({
    configurationSnapshotId: z.number().int().positive(),
    providerType: z.enum(["http-json", "kserve-v2", "ogc-processes", "ai-sdk"]),
    modelOrProcess: z.string().min(1),
    executionMode: z.enum(["synchronous", "asynchronous", "callback"]),
    mappingVersion: z.string().min(1)
  }),
  inputAsset: assetReferenceSchema,
  context: spatiotemporalContextSchema,
  parameters: z.record(z.string(), z.unknown()),
  callback: z.object({ url: z.url(), token: z.string().min(32) }).nullable()
}).superRefine((input, context) => {
  if (input.definition.executionMode === "callback" && !input.callback) {
    context.addIssue({ code: "custom", path: ["callback"], message: "callback execution requires callback credentials" });
  }
  if (input.definition.executionMode !== "callback" && input.callback) {
    context.addIssue({ code: "custom", path: ["callback"], message: "callback credentials are only valid in callback mode" });
  }
});

const pixelGeometrySchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("bbox"), x: z.number().nonnegative(), y: z.number().nonnegative(), width: z.number().positive(), height: z.number().positive() }),
  z.object({ type: z.literal("polygon"), coordinates: z.array(z.tuple([z.number().nonnegative(), z.number().nonnegative()])).min(3) })
]);

const geographicGeometrySchema = z.object({
  type: z.literal("Polygon"),
  coordinates: z.array(z.array(z.tuple([z.number().min(-180).max(180), z.number().min(-90).max(90)])).min(4)).min(1),
  quality: z.enum(["surveyed", "estimated", "low", "unavailable"]),
  method: z.string().min(1),
  horizontalErrorMeters: z.number().nonnegative(),
  transformVersion: z.string().min(1)
});

export const canonicalAlgorithmResultSchema = z.object({
  schemaVersion: z.literal("aerosight.algorithm.result/v1"),
  runId: z.uuid(),
  source: z.object({
    providerType: z.enum(["http-json", "kserve-v2", "ogc-processes", "ai-sdk"]),
    providerId: z.number().int().positive(),
    modelOrProcess: z.string().min(1),
    modelRevision: z.string().min(1).nullable(),
    modelDigest: z.string().min(1).nullable(),
    configurationSnapshotId: z.number().int().positive(),
    mappingVersion: z.string().min(1)
  }),
  inputAsset: assetReferenceSchema.omit({ accessUrl: true, accessExpiresAt: true }),
  capturedAt: z.iso.datetime(),
  detections: z.array(z.object({
    detectionKey: z.string().min(1),
    label: z.string().min(1),
    confidence: z.number().min(0).max(1),
    pixelGeometry: pixelGeometrySchema,
    geographicGeometry: geographicGeometrySchema.nullable(),
    attributes: z.record(z.string(), z.unknown()),
    derivedAssetIds: z.array(z.number().int().positive())
  })),
  rawResult: z.object({
    objectKey: z.string().min(1),
    checksumSha256: z.string().regex(/^[a-f0-9]{64}$/),
    contentType: z.string().min(1)
  }),
  completedAt: z.iso.datetime()
});

export const algorithmAdapterInputJsonSchema = z.toJSONSchema(algorithmAdapterInputSchema, { target: "draft-2020-12" });
export const canonicalAlgorithmResultJsonSchema = z.toJSONSchema(canonicalAlgorithmResultSchema, { target: "draft-2020-12" });

export type AlgorithmAdapterInput = z.infer<typeof algorithmAdapterInputSchema>;
export type CanonicalAlgorithmResult = z.infer<typeof canonicalAlgorithmResultSchema>;
export type AlgorithmAdapterOutcome =
  | { kind: "completed"; result: CanonicalAlgorithmResult }
  | { kind: "accepted"; externalJobId: string; nextPollAt: string }
  | { kind: "waiting_callback"; externalJobId: string };

export interface AlgorithmAdapter {
  readonly providerType: AlgorithmAdapterInput["definition"]["providerType"];
  execute(input: AlgorithmAdapterInput, signal: AbortSignal): Promise<AlgorithmAdapterOutcome>;
  poll?(externalJobId: string, input: AlgorithmAdapterInput, signal: AbortSignal): Promise<AlgorithmAdapterOutcome>;
  receiveCallback?(payload: unknown, input: AlgorithmAdapterInput): Promise<CanonicalAlgorithmResult>;
}
