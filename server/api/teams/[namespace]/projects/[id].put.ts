import { z } from "zod";

// 定义项目更新参数验证模式
const updateProjectSchema = z.object({
  name: z.string().min(1, "项目名称不能为空"),
  description: z.string().optional(),
});

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const namespace = event.context.params?.namespace;
  const id = parseInt(event.context.params?.id || '');

  if (isNaN(id)) {
    throw createError({
      statusCode: 400,
      message: '无效的项目ID',
    });
  }

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
  const body = await readValidatedBody(event, updateProjectSchema.parse);
  
  // 更新项目
  const project = await prisma.project.update({
    where: {
      id,
      teamId: team.id,
    },
    data: {
      name: body.name,
      description: body.description,
    },
  });

  return project;
});
