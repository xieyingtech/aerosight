import { z } from "zod";

const createTeamSchema = z.object({
  name: z.string().min(1, "团队名称不能为空").max(100, "团队名称过长"),
  namespace: z
    .string()
    .regex(/^[a-zA-Z0-9-]+$/, "团队ID只能包含字母、数字和连字符(-)"),
  description: z.string().optional().default(""),
});

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);

  const body = await readValidatedBody(event, createTeamSchema.parse);

  const ownerRole = await prisma.role.findFirst({
    where: { name: "Owner", teamId: null }, // 假设 "Owner" 是一个全局角色
  });

  if (!ownerRole) {
    // 如果找不到 "Owner" 角色，则抛出错误，因为团队创建者应具有此角色
    throw createError({
      statusCode: 500,
      message: "Default 'Owner' role not found. Please ensure it is created.",
    });
  }

  return await prisma.team.create({
    data: {
      name: body.name,
      namespace: body.namespace,
      description: body.description,
      teamMembers: {
        create: {
          userId: user.id,
          memberRoles: {
            create: {
              role: { connect: { id: ownerRole.id } },
            },
          },
        },
      },
    },
    include: {
      teamMembers: {
        include: {
          user: true,
          memberRoles: {
            include: {
              role: true,
            },
          },
        },
      },
    },
  });
});
