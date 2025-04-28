import { requireAdminSession } from '~/server/utils/auth'

export default defineEventHandler(async (event) => {
  await requireAdminSession(event);
  
  const body = await readBody(event)
  
  if (!body.name) {
    throw createError({
      statusCode: 400,
      message: 'Organization name is required'
    })
  }

  const newOrg = await prisma.organization.create({
    data: {
      name: body.name
    }
  })

  return newOrg
})
