export type SituationSelection = { lane: string; entityId: string; label: string; timestamp?: string };
export type SituationState = {
  mode: "live" | "history";
  selection: SituationSelection | null;
  cursor: string | null;
  range: { from: string; to: string } | null;
};
export type SituationAction =
  | { type: "select"; selection: SituationSelection }
  | { type: "set-cursor"; cursor: string }
  | { type: "set-range"; from: string; to: string }
  | { type: "return-live" };

export const initialSituationState: SituationState = { mode: "live", selection: null, cursor: null, range: null };

export function situationReducer(state: SituationState, action: SituationAction): SituationState {
  switch (action.type) {
    case "select":
      return { ...state, selection: action.selection, cursor: action.selection.timestamp ?? state.cursor };
    case "set-cursor":
      return { ...state, mode: "history", cursor: action.cursor };
    case "set-range": {
      const from = Date.parse(action.from) <= Date.parse(action.to) ? action.from : action.to;
      const to = Date.parse(action.from) <= Date.parse(action.to) ? action.to : action.from;
      return { ...state, mode: "history", range: { from, to }, cursor: to };
    }
    case "return-live":
      return initialSituationState;
  }
}

export function interpolateTimeline(from: string, to: string, position: number, maximum = 1000) {
  const ratio = Math.max(0, Math.min(maximum, position)) / maximum;
  return new Date(Date.parse(from) + (Date.parse(to) - Date.parse(from)) * ratio).toISOString();
}
