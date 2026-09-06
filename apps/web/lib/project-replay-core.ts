export type ReplayQuery = {
  from: string;
  to: string;
  deviceTypes: string[];
  bbox: [number, number, number, number] | null;
};

export type ProjectReplay = {
  projectId: number;
  mode: "replay";
  window: { from: string; to: string };
  filters: { deviceTypes: string[]; bbox: ReplayQuery["bbox"] };
  poses: Array<Record<string, unknown>>;
  media: Array<Record<string, unknown>>;
  events: Array<Record<string, unknown>>;
  truncated: boolean;
};
