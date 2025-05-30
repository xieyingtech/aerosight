import { z } from "zod";

const createTeamSchema = z.object({
  name: z.string().min(1, "团队名称不能为空").max(100, "团队名称过长"),
  description: z.string().optional(),
});

export default defineEventHandler(async (event) => {
  // 直接验证用户是否为管理员
  const { user } = await requireAdminSession(event);

  const body = await readValidatedBody(event, createTeamSchema.parse);

  // 管理员创建团队，但不自动加入团队
  return await prisma.team.create({
    data: {
      name: body.name,
      description: body.description,
    },
    include: {
      teamMembers: true, // Changed from memberships
    },
  });
});
