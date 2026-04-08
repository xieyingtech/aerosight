import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
	const projectId = Number(getRouterParam(event, "id"));
	await requireProjectMembership(event, projectId);

	return db
		.select({
			id: schema.tasks.id,
			name: schema.tasks.name,
			description: schema.tasks.description,
			triggerType: schema.tasks.triggerType,
			status: schema.tasks.status,
			updatedAt: schema.tasks.updatedAt,
		})
		.from(schema.tasks)
		.where(eq(schema.tasks.projectId, projectId))
		.orderBy(desc(schema.tasks.updatedAt));
});