import { z } from "zod";

// 布防点创建数据验证
const createPoiSchema = z.object({
  name: z.string().min(1, "布防点名称不能为空"),
  description: z.string().optional().default(""),
  latitude: z.number().min(-90).max(90),
  longitude: z.number().min(-180).max(180),
});

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const namespace = event.context.params?.namespace;
  const projectId = parseInt(event.context.params?.id || '');

  if (isNaN(projectId)) {
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
  const membership = await prisma.membership.findFirst({
    where: {
      userId: user.id,
      teamId: team.id,
    },
  });

  if (!membership && !user.admin) {
    throw createError({
      statusCode: 403,
      message: '您不是该团队成员',
    });
  }

  // 检查项目是否存在且属于该团队
  const project = await prisma.project.findFirst({
    where: {
      id: projectId,
      teamId: team.id,
    },
  });

  if (!project) {
    throw createError({
      statusCode: 404,
      message: '项目不存在',
    });
  }

  // 验证请求数据
  const body = await readValidatedBody(event, createPoiSchema.parse);

  // 创建布防点
  const poi = await prisma.pOI.create({
    data: {
      name: body.name,
      description: body.description,
      latitude: body.latitude,
      longitude: body.longitude,
      projectId,
    },
  });

  return poi;
});
