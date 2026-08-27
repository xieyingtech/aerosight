export type DetectionMapping = {
  detectionsPath: string;
  keyPath: string;
  labelPath: string;
  confidencePath: string;
  geometryPath: string;
  geometryTypePath?: string;
  geometryFormat: "object" | "bbox-array" | "polygon-array";
};

export type DetectionAssetReference = {
  assetId: number;
  version: number;
  checksumSha256: string;
  mimeType: string;
};

export const suspectedConstructionTemplate = {
  templateKey: "suspected-construction",
  templateVersion: 1,
  name: "疑似违建识别",
  description: "识别新增、扩建等疑似建设活动；结果是机器线索，不代表法律结论。",
  capabilityCode: "perception.suspected-construction",
  executionMode: "synchronous",
  inputRequirements: { assetKinds: ["image"], mimeTypes: ["image/jpeg", "image/png"] },
  parametersSchema: {
    type: "object", additionalProperties: false,
    properties: { threshold: { type: "number", minimum: 0, maximum: 1, default: 0.65 } }
  },
  outputMapping: {
    detectionsPath: "results", keyPath: "id", labelPath: "class", confidencePath: "score",
    geometryPath: "geometry", geometryTypePath: "geometry.type", geometryFormat: "object"
  } satisfies DetectionMapping,
  labelMapping: {
    new_building: "suspected-construction:new-building",
    extension: "suspected-construction:extension",
    earthwork: "suspected-construction:earthwork",
    suspected_construction: "suspected-construction"
  },
  publishThreshold: 0.65,
  mappingVersion: "suspected-construction/v1"
} as const;

export function mapSuspectedConstructionDetections(input: {
  response: unknown;
  mapping: DetectionMapping;
  labelMapping: Readonly<Record<string, string>>;
  inputAsset: DetectionAssetReference;
}) {
  const collection = valueAt(input.response, input.mapping.detectionsPath);
  if (!Array.isArray(collection)) throw new Error(`DETECTIONS_PATH_INVALID:${input.mapping.detectionsPath}`);
  return collection.map((item, index) => {
    const externalLabel = valueAt(item, input.mapping.labelPath);
    const confidence = valueAt(item, input.mapping.confidencePath);
    const key = valueAt(item, input.mapping.keyPath);
    if (typeof externalLabel !== "string" || typeof confidence !== "number" || confidence < 0 || confidence > 1) {
      throw new Error(`DETECTION_MAPPING_INVALID:${index}:label-or-confidence`);
    }
    const label = input.labelMapping[externalLabel];
    if (!label) throw new Error(`DETECTION_LABEL_UNMAPPED:${externalLabel}`);
    return {
      detectionKey: typeof key === "string" && key ? key : String(index),
      label,
      confidence,
      pixelGeometry: mapPixelGeometry(item, input.mapping, index),
      inputAsset: { ...input.inputAsset },
      attributes: { externalLabel }
    };
  });
}

function mapPixelGeometry(item: unknown, mapping: DetectionMapping, index: number) {
  const geometry = valueAt(item, mapping.geometryPath);
  if (mapping.geometryFormat === "bbox-array") {
    if (!isNumberArray(geometry, 4)) throw new Error(`DETECTION_MAPPING_INVALID:${index}:bbox`);
    const [x, y, width, height] = geometry;
    if (x < 0 || y < 0 || width <= 0 || height <= 0) throw new Error(`DETECTION_MAPPING_INVALID:${index}:bbox`);
    return { type: "bbox" as const, x, y, width, height };
  }
  if (mapping.geometryFormat === "polygon-array") {
    if (!Array.isArray(geometry) || geometry.length < 3 || !geometry.every((point) => isNumberArray(point, 2) && point[0] >= 0 && point[1] >= 0)) {
      throw new Error(`DETECTION_MAPPING_INVALID:${index}:polygon`);
    }
    return { type: "polygon" as const, coordinates: geometry as [number, number][] };
  }
  if (!geometry || typeof geometry !== "object" || Array.isArray(geometry)) throw new Error(`DETECTION_MAPPING_INVALID:${index}:geometry`);
  const object = geometry as Record<string, unknown>;
  const type = mapping.geometryTypePath ? valueAt(item, mapping.geometryTypePath) : object.type;
  if (type === "bbox" && [object.x, object.y, object.width, object.height].every((value) => typeof value === "number")) {
    const { x, y, width, height } = object as { x: number; y: number; width: number; height: number };
    if (x >= 0 && y >= 0 && width > 0 && height > 0) return { type: "bbox" as const, x, y, width, height };
  }
  if (type === "polygon" && Array.isArray(object.coordinates) && object.coordinates.length >= 3 && object.coordinates.every((point) => isNumberArray(point, 2))) {
    return { type: "polygon" as const, coordinates: object.coordinates as [number, number][] };
  }
  throw new Error(`DETECTION_MAPPING_INVALID:${index}:geometry`);
}

function valueAt(value: unknown, path: string): unknown {
  let current = value;
  for (const part of path.split(".")) {
    if (!current || typeof current !== "object" || Array.isArray(current)) return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

function isNumberArray(value: unknown, length: number): value is number[] {
  return Array.isArray(value) && value.length === length && value.every((item) => typeof item === "number" && Number.isFinite(item));
}
