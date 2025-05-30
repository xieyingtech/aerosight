import { z } from "zod";

// 定义项目创建参数验证模式
const createProjectSchema = z.object({
  name: z.string().min(1, "项目名称不能为空"),
  description: z.string().optional().default(""),
});

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const namespace = event.context.params?.namespace;

  // 查找团队
  const team = await prisma.team.findUnique({
    where: { namespace },
  });

  if (!team) {
    throw createError({
      statusCode: 404,
      message: '团队不存在',
    });
  }

  // 检查用户是否是团队成员
  const teamMember = await prisma.teamMember.findFirst({
    where: {
      userId: user.id,
      teamId: team.id,
    },
  });

  // Fetch the full user object to check systemAdmin status
  const fullUser = await prisma.user.findUnique({ where: { id: user.id } });

  if (!teamMember && !fullUser?.systemAdmin) {
    throw createError({
      statusCode: 403,
      message: '您不是该团队成员或系统管理员',
    });
  }

  // 验证请求数据
  const body = await readValidatedBody(event, createProjectSchema.parse);
  
  // 创建项目
  const project = await prisma.project.create({
    data: {
      name: body.name,
      description: body.description,
      teamId: team.id,
    },
  });

  return project;
});
