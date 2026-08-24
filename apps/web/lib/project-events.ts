import "server-only";

import type { PoolClient } from "pg";

export type ProjectEventInput = {
  projectId: number;
  teamId: number;
  eventId: string;
  eventType: string;
  payload?: Record<string, unknown>;
  occurredAt?: Date;
  enqueue?: boolean;
  maxAttempts?: number;
};

export async function publishProjectEvent(client: PoolClient, event: ProjectEventInput) {
  const published = await client.query<{ cursor: string }>(
    `insert into project_events (
       project_id, team_id, event_id, event_type, payload_json, occurred_at
     ) values ($1, $2, $3, $4, $5, coalesce($6, now()))
     on conflict (event_id) do update set event_id = excluded.event_id
     returning cursor`,
    [
      event.projectId,
      event.teamId,
      event.eventId,
      event.eventType,
      event.payload ?? {},
      event.occurredAt ?? null
    ]
  );

  if (event.enqueue !== false) {
    await client.query(
      `insert into outbox_events (
         project_id, team_id, event_id, event_type, payload_json, max_attempts
       ) values ($1, $2, $3, $4, $5, $6)
       on conflict (event_id) do nothing`,
      [
        event.projectId,
        event.teamId,
        event.eventId,
        event.eventType,
        event.payload ?? {},
        event.maxAttempts ?? 8
      ]
    );
  }

  return Number(published.rows[0].cursor);
}
