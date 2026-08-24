export class ReplayControlForbiddenError extends Error {
  readonly code = "REPLAY_CONTROL_FORBIDDEN";
  constructor() { super("device control is forbidden in replay mode"); }
}

export function requestOperationMode(request: Request) {
  const header = request.headers.get("X-AeroSight-Mode");
  const query = new URL(request.url).searchParams.get("mode");
  return header === "replay" || query === "replay" ? "replay" : "live";
}

export function assertLiveControlRequest(request: Request) {
  if (requestOperationMode(request) === "replay") throw new ReplayControlForbiddenError();
}
