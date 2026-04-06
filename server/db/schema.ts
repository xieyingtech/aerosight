import { relations, sql } from "drizzle-orm";
import {
  bigint,
  index,
  integer,
  jsonb,
  pgTable,
  serial,
  text,
  timestamp,
  uniqueIndex,
} from "drizzle-orm/pg-core";

export const users = pgTable("users", {
  id: serial("id").primaryKey(),
  name: text("name").notNull(),
  email: text("email").unique(),
  phone: text("phone").unique(),
  password: text("password"),
  role: text("role", { enum: ["user", "admin"] })
    .notNull()
    .default("user"),
  createdAt: timestamp("created_at").notNull().defaultNow(),
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
});

export const teams = pgTable("teams", {
  id: serial("id").primaryKey(),
  name: text("name").notNull(),
  createdAt: timestamp("created_at").notNull().defaultNow(),
  updatedAt: timestamp("updated_at").notNull().defaultNow(),
});

export const teamMembers = pgTable(
  "team_members",
  {
    id: serial("id").primaryKey(),
    teamId: integer("team_id")
      .notNull()
      .references(() => teams.id, { onDelete: "cascade" }),
    userId: integer("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    role: text("role", { enum: ["owner", "admin", "member"] })
      .notNull()
      .default("member"),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("team_members_team_user_unique").on(table.teamId, table.userId),
    uniqueIndex("team_members_single_owner_unique")
      .on(table.teamId)
      .where(sql`${table.role} = 'owner'`),
    index("team_members_user_idx").on(table.userId),
  ],
);

export const projects = pgTable(
  "projects",
  {
    id: serial("id").primaryKey(),
    teamId: integer("team_id")
      .notNull()
      .references(() => teams.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    description: text("description"),
    createdByUserId: integer("created_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    createdAt: timestamp("created_at").notNull().defaultNow(),
    updatedAt: timestamp("updated_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("projects_team_name_unique").on(table.teamId, table.name),
    index("projects_team_idx").on(table.teamId),
  ],
);

export const devices = pgTable(
  "devices",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    type: text("type").notNull(),
    status: text("status", { enum: ["offline", "online", "busy", "error"] })
      .notNull()
      .default("offline"),
    lastSeenAt: timestamp("last_seen_at"),
    configJson: jsonb("config_json").notNull().default({}),
    metadataJson: jsonb("metadata_json").notNull().default({}),
    createdAt: timestamp("created_at").notNull().defaultNow(),
    updatedAt: timestamp("updated_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("devices_project_name_unique").on(table.projectId, table.name),
    index("devices_project_status_idx").on(table.projectId, table.status),
    index("devices_last_seen_idx").on(table.lastSeenAt),
  ],
);

export const agents = pgTable(
  "agents",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    description: text("description"),
    status: text("status", {
      enum: ["disabled", "idle", "running", "error"],
    })
      .notNull()
      .default("disabled"),
    configJson: jsonb("config_json").notNull().default({}),
    createdAt: timestamp("created_at").notNull().defaultNow(),
    updatedAt: timestamp("updated_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("agents_project_name_unique").on(table.projectId, table.name),
    index("agents_project_status_idx").on(table.projectId, table.status),
  ],
);

export const deviceCapabilities = pgTable(
  "device_capabilities",
  {
    id: serial("id").primaryKey(),
    deviceId: integer("device_id")
      .notNull()
      .references(() => devices.id, { onDelete: "cascade" }),
    capabilityCode: text("capability_code").notNull(),
    version: text("version"),
    paramsSchemaJson: jsonb("params_schema_json").notNull().default({}),
    constraintsJson: jsonb("constraints_json").notNull().default({}),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("device_capabilities_device_code_unique").on(
      table.deviceId,
      table.capabilityCode,
    ),
    index("device_capabilities_code_idx").on(table.capabilityCode),
  ],
);

export const tasks = pgTable(
  "tasks",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    name: text("name").notNull(),
    description: text("description"),
    triggerType: text("trigger_type", {
      enum: ["manual", "schedule", "event"],
    }).notNull(),
    status: text("status", { enum: ["active", "paused", "archived"] })
      .notNull()
      .default("active"),
    requiredCapabilityCode: text("required_capability_code"),
    targetSelectorJson: jsonb("target_selector_json").notNull().default({}),
    schedule: text("schedule"),
    eventRuleJson: jsonb("event_rule_json").notNull().default({}),
    script: text("script").notNull(),
    createdByUserId: integer("created_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    createdAt: timestamp("created_at").notNull().defaultNow(),
    updatedAt: timestamp("updated_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("tasks_project_name_unique").on(table.projectId, table.name),
    index("tasks_project_status_idx").on(table.projectId, table.status),
    index("tasks_trigger_type_idx").on(table.triggerType),
  ],
);

export const taskRuns = pgTable(
  "task_runs",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    taskId: integer("task_id")
      .notNull()
      .references(() => tasks.id, { onDelete: "cascade" }),
    triggerSource: text("trigger_source", {
      enum: ["manual", "schedule", "event", "retry"],
    }).notNull(),
    status: text("status", {
      enum: ["queued", "running", "succeeded", "failed", "cancelled"],
    })
      .notNull()
      .default("queued"),
    inputSnapshotJson: jsonb("input_snapshot_json").notNull().default({}),
    outputSnapshotJson: jsonb("output_snapshot_json").notNull().default({}),
    errorMessage: text("error_message"),
    startedAt: timestamp("started_at"),
    finishedAt: timestamp("finished_at"),
    createdByUserId: integer("created_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    index("task_runs_project_created_idx").on(table.projectId, table.createdAt),
    index("task_runs_task_created_idx").on(table.taskId, table.createdAt),
    index("task_runs_status_idx").on(table.status),
  ],
);

export const issues = pgTable(
  "issues",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    number: integer("number").notNull(),
    title: text("title").notNull(),
    description: text("description"),
    sourceType: text("source_type", {
      enum: ["human", "agent", "task_run"],
    }).notNull(),
    sourceId: integer("source_id"),
    status: text("status", {
      enum: ["open", "in_progress", "resolved", "closed"],
    })
      .notNull()
      .default("open"),
    priority: text("priority", {
      enum: ["low", "medium", "high", "critical"],
    })
      .notNull()
      .default("medium"),
    taskRunId: integer("task_run_id").references(() => taskRuns.id, {
      onDelete: "set null",
    }),
    openedByUserId: integer("opened_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    assigneeUserId: integer("assignee_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    closedAt: timestamp("closed_at"),
    createdAt: timestamp("created_at").notNull().defaultNow(),
    updatedAt: timestamp("updated_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("issues_project_number_unique").on(
      table.projectId,
      table.number,
    ),
    index("issues_project_status_idx").on(table.projectId, table.status),
    index("issues_project_priority_idx").on(table.projectId, table.priority),
    index("issues_task_run_idx").on(table.taskRunId),
  ],
);

export const assets = pgTable(
  "assets",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    deviceId: integer("device_id").references(() => devices.id, {
      onDelete: "set null",
    }),
    taskRunId: integer("task_run_id").references(() => taskRuns.id, {
      onDelete: "set null",
    }),
    issueId: integer("issue_id").references(() => issues.id, {
      onDelete: "set null",
    }),
    kind: text("kind", {
      enum: ["image", "video", "file", "timeseries"],
    }).notNull(),
    mimeType: text("mime_type"),
    storageKey: text("storage_key").notNull(),
    sizeBytes: bigint("size_bytes", { mode: "number" }),
    checksum: text("checksum"),
    capturedAt: timestamp("captured_at"),
    metadataJson: jsonb("metadata_json").notNull().default({}),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    index("assets_project_created_idx").on(table.projectId, table.createdAt),
    index("assets_task_run_idx").on(table.taskRunId),
    index("assets_issue_idx").on(table.issueId),
    index("assets_device_idx").on(table.deviceId),
  ],
);

export const agentSessions = pgTable(
  "agent_sessions",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    agentId: integer("agent_id").references(() => agents.id, {
      onDelete: "set null",
    }),
    taskRunId: integer("task_run_id").references(() => taskRuns.id, {
      onDelete: "set null",
    }),
    issueId: integer("issue_id").references(() => issues.id, {
      onDelete: "set null",
    }),
    status: text("status", {
      enum: ["open", "running", "completed", "failed", "aborted"],
    })
      .notNull()
      .default("open"),
    startedByUserId: integer("started_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    summary: text("summary"),
    startedAt: timestamp("started_at").notNull().defaultNow(),
    endedAt: timestamp("ended_at"),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    index("agent_sessions_project_created_idx").on(
      table.projectId,
      table.createdAt,
    ),
    index("agent_sessions_issue_idx").on(table.issueId),
    index("agent_sessions_task_run_idx").on(table.taskRunId),
  ],
);

export const agentMessages = pgTable(
  "agent_messages",
  {
    id: serial("id").primaryKey(),
    sessionId: integer("session_id")
      .notNull()
      .references(() => agentSessions.id, { onDelete: "cascade" }),
    role: text("role", {
      enum: ["system", "user", "assistant", "tool"],
    }).notNull(),
    content: text("content").notNull(),
    toolCallsJson: jsonb("tool_calls_json").notNull().default({}),
    tokenUsageJson: jsonb("token_usage_json").notNull().default({}),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    index("agent_messages_session_created_idx").on(
      table.sessionId,
      table.createdAt,
    ),
  ],
);

export const issueEvents = pgTable(
  "issue_events",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    issueId: integer("issue_id")
      .notNull()
      .references(() => issues.id, { onDelete: "cascade" }),
    eventType: text("event_type", {
      enum: [
        "opened",
        "commented",
        "status_changed",
        "assigned",
        "unassigned",
        "linked",
        "unlinked",
        "agent_note",
        "automation_action",
        "closed",
        "reopened",
      ],
    }).notNull(),
    body: text("body"),
    metadataJson: jsonb("metadata_json").notNull().default({}),
    actorUserId: integer("actor_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    actorAgentId: integer("actor_agent_id").references(() => agents.id, {
      onDelete: "set null",
    }),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    index("issue_events_issue_created_idx").on(table.issueId, table.createdAt),
    index("issue_events_project_created_idx").on(
      table.projectId,
      table.createdAt,
    ),
  ],
);

export const issueLinks = pgTable(
  "issue_links",
  {
    id: serial("id").primaryKey(),
    projectId: integer("project_id")
      .notNull()
      .references(() => projects.id, { onDelete: "cascade" }),
    issueId: integer("issue_id")
      .notNull()
      .references(() => issues.id, { onDelete: "cascade" }),
    linkType: text("link_type", {
      enum: ["asset", "task", "task_run", "device", "agent_session"],
    }).notNull(),
    targetId: integer("target_id").notNull(),
    createdByUserId: integer("created_by_user_id").references(() => users.id, {
      onDelete: "set null",
    }),
    createdAt: timestamp("created_at").notNull().defaultNow(),
  },
  (table) => [
    uniqueIndex("issue_links_issue_target_unique").on(
      table.issueId,
      table.linkType,
      table.targetId,
    ),
    index("issue_links_target_idx").on(table.linkType, table.targetId),
    index("issue_links_project_issue_idx").on(table.projectId, table.issueId),
  ],
);

export const usersRelations = relations(users, ({ many }) => ({
  teamMemberships: many(teamMembers),
  createdProjects: many(projects),
  createdTasks: many(tasks),
  createdTaskRuns: many(taskRuns),
  openedIssues: many(issues, { relationName: "issues_opened_by_user" }),
  assignedIssues: many(issues, { relationName: "issues_assigned_to_user" }),
  startedAgentSessions: many(agentSessions),
  issueEvents: many(issueEvents),
  issueLinks: many(issueLinks),
}));

export const teamsRelations = relations(teams, ({ many }) => ({
  members: many(teamMembers),
  projects: many(projects),
}));

export const teamMembersRelations = relations(teamMembers, ({ one }) => ({
  team: one(teams, {
    fields: [teamMembers.teamId],
    references: [teams.id],
  }),
  user: one(users, {
    fields: [teamMembers.userId],
    references: [users.id],
  }),
}));

export const projectsRelations = relations(projects, ({ one, many }) => ({
  team: one(teams, {
    fields: [projects.teamId],
    references: [teams.id],
  }),
  createdByUser: one(users, {
    fields: [projects.createdByUserId],
    references: [users.id],
  }),
  devices: many(devices),
  agents: many(agents),
  tasks: many(tasks),
  taskRuns: many(taskRuns),
  issues: many(issues),
  assets: many(assets),
  agentSessions: many(agentSessions),
  issueEvents: many(issueEvents),
  issueLinks: many(issueLinks),
}));

export const devicesRelations = relations(devices, ({ one, many }) => ({
  project: one(projects, {
    fields: [devices.projectId],
    references: [projects.id],
  }),
  capabilities: many(deviceCapabilities),
  assets: many(assets),
}));

export const agentsRelations = relations(agents, ({ one, many }) => ({
  project: one(projects, {
    fields: [agents.projectId],
    references: [projects.id],
  }),
  sessions: many(agentSessions),
  issueEvents: many(issueEvents),
}));

export const deviceCapabilitiesRelations = relations(
  deviceCapabilities,
  ({ one }) => ({
    device: one(devices, {
      fields: [deviceCapabilities.deviceId],
      references: [devices.id],
    }),
  }),
);

export const tasksRelations = relations(tasks, ({ one, many }) => ({
  project: one(projects, {
    fields: [tasks.projectId],
    references: [projects.id],
  }),
  createdByUser: one(users, {
    fields: [tasks.createdByUserId],
    references: [users.id],
  }),
  runs: many(taskRuns),
}));

export const taskRunsRelations = relations(taskRuns, ({ one, many }) => ({
  project: one(projects, {
    fields: [taskRuns.projectId],
    references: [projects.id],
  }),
  task: one(tasks, {
    fields: [taskRuns.taskId],
    references: [tasks.id],
  }),
  createdByUser: one(users, {
    fields: [taskRuns.createdByUserId],
    references: [users.id],
  }),
  issues: many(issues),
  assets: many(assets),
  agentSessions: many(agentSessions),
}));

export const issuesRelations = relations(issues, ({ one, many }) => ({
  project: one(projects, {
    fields: [issues.projectId],
    references: [projects.id],
  }),
  taskRun: one(taskRuns, {
    fields: [issues.taskRunId],
    references: [taskRuns.id],
  }),
  openedByUser: one(users, {
    fields: [issues.openedByUserId],
    references: [users.id],
    relationName: "issues_opened_by_user",
  }),
  assigneeUser: one(users, {
    fields: [issues.assigneeUserId],
    references: [users.id],
    relationName: "issues_assigned_to_user",
  }),
  events: many(issueEvents),
  links: many(issueLinks),
  assets: many(assets),
  agentSessions: many(agentSessions),
}));

export const assetsRelations = relations(assets, ({ one }) => ({
  project: one(projects, {
    fields: [assets.projectId],
    references: [projects.id],
  }),
  device: one(devices, {
    fields: [assets.deviceId],
    references: [devices.id],
  }),
  taskRun: one(taskRuns, {
    fields: [assets.taskRunId],
    references: [taskRuns.id],
  }),
  issue: one(issues, {
    fields: [assets.issueId],
    references: [issues.id],
  }),
}));

export const agentSessionsRelations = relations(
  agentSessions,
  ({ one, many }) => ({
    project: one(projects, {
      fields: [agentSessions.projectId],
      references: [projects.id],
    }),
    agent: one(agents, {
      fields: [agentSessions.agentId],
      references: [agents.id],
    }),
    taskRun: one(taskRuns, {
      fields: [agentSessions.taskRunId],
      references: [taskRuns.id],
    }),
    issue: one(issues, {
      fields: [agentSessions.issueId],
      references: [issues.id],
    }),
    startedByUser: one(users, {
      fields: [agentSessions.startedByUserId],
      references: [users.id],
    }),
    messages: many(agentMessages),
  }),
);

export const agentMessagesRelations = relations(agentMessages, ({ one }) => ({
  session: one(agentSessions, {
    fields: [agentMessages.sessionId],
    references: [agentSessions.id],
  }),
}));

export const issueEventsRelations = relations(issueEvents, ({ one }) => ({
  project: one(projects, {
    fields: [issueEvents.projectId],
    references: [projects.id],
  }),
  issue: one(issues, {
    fields: [issueEvents.issueId],
    references: [issues.id],
  }),
  actorUser: one(users, {
    fields: [issueEvents.actorUserId],
    references: [users.id],
  }),
  actorAgent: one(agents, {
    fields: [issueEvents.actorAgentId],
    references: [agents.id],
  }),
}));

export const issueLinksRelations = relations(issueLinks, ({ one }) => ({
  project: one(projects, {
    fields: [issueLinks.projectId],
    references: [projects.id],
  }),
  issue: one(issues, {
    fields: [issueLinks.issueId],
    references: [issues.id],
  }),
  createdByUser: one(users, {
    fields: [issueLinks.createdByUserId],
    references: [users.id],
  }),
}));
