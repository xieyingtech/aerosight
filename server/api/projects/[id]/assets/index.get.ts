import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
	const projectId = Number(getRouterParam(event, "id"));
	await requireProjectMembership(event, projectId);

	return db
		.select({
			id: schema.assets.id,
			kind: schema.assets.kind,
			mimeType: schema.assets.mimeType,
			capturedAt: schema.assets.capturedAt,
			createdAt: schema.assets.createdAt,
		})
		.from(schema.assets)
		.where(eq(schema.assets.projectId, projectId))
		.orderBy(desc(schema.assets.createdAt));
});