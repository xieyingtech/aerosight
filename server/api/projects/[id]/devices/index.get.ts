import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
	const projectId = Number(getRouterParam(event, "id"));
	await requireProjectMembership(event, projectId);

	return db
		.select({
			id: schema.devices.id,
			name: schema.devices.name,
			type: schema.devices.type,
			status: schema.devices.status,
			lastSeenAt: schema.devices.lastSeenAt,
			updatedAt: schema.devices.updatedAt,
		})
		.from(schema.devices)
		.where(eq(schema.devices.projectId, projectId))
		.orderBy(desc(schema.devices.updatedAt));
});