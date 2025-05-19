import { z } from "zod";

const createTeamSchema = z.object({
  name: z.string().min(1, "团队名称不能为空").max(100, "团队名称过长"),
  description: z.string().optional(),
});

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const body = await readValidatedBody(event, createTeamSchema.parse);

  return await prisma.team.create({
    data: {
      name: body.name,
      description: body.description,
      // 同时创建创建者的成员身份，设置为管理员角色
      memberships: {
        create: {
          userId: user.id,
          roles: ["admin"],
        },
      },
    },
    include: {
      memberships: true,
    },
  });
});
