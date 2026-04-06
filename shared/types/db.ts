import { projects, teamMembers } from "@nuxthub/db/schema";

export type ProjectRole = (typeof teamMembers.role.enumValues)[number];

export type Project = typeof projects.$inferSelect;
