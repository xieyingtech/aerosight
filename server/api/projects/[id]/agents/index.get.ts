import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
	const projectId = Number(getRouterParam(event, "id"));
	await requireProjectMembership(event, projectId);

	return db
		.select({
			id: schema.agents.id,
			name: schema.agents.name,
			description: schema.agents.description,
			status: schema.agents.status,
			updatedAt: schema.agents.updatedAt,
		})
		.from(schema.agents)
		.where(eq(schema.agents.projectId, projectId))
		.orderBy(desc(schema.agents.updatedAt));
});