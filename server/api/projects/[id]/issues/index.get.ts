import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
	const projectId = Number(getRouterParam(event, "id"));
	await requireProjectMembership(event, projectId);

	return db
		.select({
			id: schema.issues.id,
			number: schema.issues.number,
			title: schema.issues.title,
			status: schema.issues.status,
			priority: schema.issues.priority,
			updatedAt: schema.issues.updatedAt,
		})
		.from(schema.issues)
		.where(eq(schema.issues.projectId, projectId))
		.orderBy(desc(schema.issues.updatedAt));
});