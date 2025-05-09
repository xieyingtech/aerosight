import { consola } from "consola";

export default defineNitroPlugin(async (nitroApp) => {
  const adminCount = await prisma.user.count({ where: { admin: true } });
  if (adminCount === 0) {
    prisma.user
      .create({
        data: {
          username: process.env.ADMIN_USERNAME || "admin",
          password: await hashPassword(process.env.ADMIN_PASSWORD || "admin"),
          email: process.env.ADMIN_EMAIL,
          admin: true,
        },
      })
      .then(() => {
        consola.success("Default admin user created successfully");
      });
  }
});
