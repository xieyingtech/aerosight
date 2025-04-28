import { requireAdminSession } from '~/server/utils/auth'

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  
  const body = await readBody(event)
  
  if (!body.name) {
    throw createError({
      statusCode: 400,
      message: 'Team name is required'
    })
  }

  const newTeam = await prisma.team.create({
    data: {
      name: body.name
    }
  })

  return newTeam
})
